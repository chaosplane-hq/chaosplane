package controller

import (
	"context"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

const finalizerName = "chaosplane.io/experiment-protection"

type ExperimentReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Registry *executor.Registry
	Logger   *slog.Logger
}

func (r *ExperimentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Logger.With("experiment", req.NamespacedName)

	var exp v1alpha1.ChaosExperiment
	if err := r.Get(ctx, req.NamespacedName, &exp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !exp.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &exp, log)
	}

	if !controllerutil.ContainsFinalizer(&exp, finalizerName) {
		controllerutil.AddFinalizer(&exp, finalizerName)
		if err := r.Update(ctx, &exp); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	switch exp.Status.Phase {
	case "", v1alpha1.PhasePending:
		return r.reconcilePending(ctx, &exp, log)
	case v1alpha1.PhaseRunning:
		return r.reconcileRunning(ctx, &exp, log)
	case v1alpha1.PhaseCompleted, v1alpha1.PhaseFailed, v1alpha1.PhaseAborted:
		return ctrl.Result{}, nil
	default:
		log.Warn("unknown phase", "phase", exp.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *ExperimentReconciler) reconcilePending(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	log.Info("transitioning to Running")

	exec, err := r.Registry.Get(exp.Spec.Action.Type)
	if err != nil {
		return r.setFailed(ctx, exp, fmt.Sprintf("no executor for action %q", exp.Spec.Action.Type))
	}

	if err := exec.Validate(exp); err != nil {
		return r.setFailed(ctx, exp, fmt.Sprintf("validation failed: %v", err))
	}

	now := metav1.Now()
	exp.Status.Phase = v1alpha1.PhaseRunning
	exp.Status.StartTime = &now
	exp.Status.ObservedGeneration = exp.Generation
	setCondition(exp, "Progressing", metav1.ConditionTrue, "ExperimentStarted", "Experiment is running")

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(exp, "Normal", "Started", "Experiment started")
	return ctrl.Result{Requeue: true}, nil
}

func (r *ExperimentReconciler) reconcileRunning(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	exec, err := r.Registry.Get(exp.Spec.Action.Type)
	if err != nil {
		return r.setFailed(ctx, exp, fmt.Sprintf("executor lost: %v", err))
	}

	if err := exec.Execute(ctx, exp); err != nil {
		log.Error("execute failed, attempting rollback", "error", err)
		_ = exec.Rollback(ctx, exp)
		return r.setFailed(ctx, exp, fmt.Sprintf("execution failed: %v", err))
	}

	if exp.Spec.Rollback != nil && exp.Spec.Rollback.Enabled {
		if err := exec.Rollback(ctx, exp); err != nil {
			return r.setFailed(ctx, exp, fmt.Sprintf("rollback failed: %v", err))
		}
	}

	return r.setCompleted(ctx, exp)
}

func (r *ExperimentReconciler) handleDeletion(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(exp, finalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("handling deletion, running rollback")

	if exp.Status.Phase == v1alpha1.PhaseRunning {
		if exec, err := r.Registry.Get(exp.Spec.Action.Type); err == nil {
			if rbErr := exec.Rollback(ctx, exp); rbErr != nil {
				log.Error("rollback during deletion failed", "error", rbErr)
			}
		}
	}

	controllerutil.RemoveFinalizer(exp, finalizerName)
	if err := r.Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ExperimentReconciler) setFailed(ctx context.Context, exp *v1alpha1.ChaosExperiment, reason string) (ctrl.Result, error) {
	now := metav1.Now()
	exp.Status.Phase = v1alpha1.PhaseFailed
	exp.Status.EndTime = &now
	setCondition(exp, "Progressing", metav1.ConditionFalse, "ExperimentFailed", reason)

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Event(exp, "Warning", "Failed", reason)
	return ctrl.Result{}, nil
}

func (r *ExperimentReconciler) setCompleted(ctx context.Context, exp *v1alpha1.ChaosExperiment) (ctrl.Result, error) {
	now := metav1.Now()
	exp.Status.Phase = v1alpha1.PhaseCompleted
	exp.Status.EndTime = &now
	setCondition(exp, "Progressing", metav1.ConditionFalse, "ExperimentCompleted", "Experiment completed successfully")

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Event(exp, "Normal", "Completed", "Experiment completed successfully")
	return ctrl.Result{}, nil
}

func setCondition(exp *v1alpha1.ChaosExperiment, condType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	for i, c := range exp.Status.Conditions {
		if c.Type == condType {
			exp.Status.Conditions[i].Status = status
			exp.Status.Conditions[i].Reason = reason
			exp.Status.Conditions[i].Message = message
			exp.Status.Conditions[i].LastTransitionTime = now
			exp.Status.Conditions[i].ObservedGeneration = exp.Generation
			return
		}
	}
	exp.Status.Conditions = append(exp.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
		ObservedGeneration: exp.Generation,
	})
}

func (r *ExperimentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ChaosExperiment{}).
		Complete(r)
}
