package controller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

const (
	finalizerName   = "chaosplane.io/experiment-protection"
	abortAnnotation = "chaosplane.io/abort"
)

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
	case v1alpha1.PhaseCompleting:
		return r.reconcileCompleting(ctx, &exp, log)
	case v1alpha1.PhaseCompleted, v1alpha1.PhaseFailed, v1alpha1.PhaseAborted:
		return ctrl.Result{}, nil
	default:
		log.Warn("unknown phase", "phase", exp.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *ExperimentReconciler) reconcilePending(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
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
	setCondition(exp, "Available", metav1.ConditionFalse, "ExperimentStarted", "Experiment is running")
	setCondition(exp, "Degraded", metav1.ConditionFalse, "ExperimentStarted", "Experiment is running")

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(exp, "Normal", "Started", "Experiment started")
	log.Info("transitioning to Running")
	return ctrl.Result{Requeue: true}, nil
}

func (r *ExperimentReconciler) reconcileRunning(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	if exp.Annotations[abortAnnotation] == "true" {
		log.Info("abort annotation detected, rolling back")
		return r.handleAbort(ctx, exp, log)
	}

	if exp.Status.StartTime == nil {
		return r.setFailed(ctx, exp, "running experiment has no startTime")
	}

	elapsed := time.Since(exp.Status.StartTime.Time)
	duration := exp.Spec.Duration.Duration
	timeout := duration * 2

	if elapsed >= timeout {
		log.Warn("experiment timed out", "elapsed", elapsed, "timeout", timeout)
		r.Recorder.Event(exp, "Warning", "Timeout", "Experiment timed out, forcing rollback")
		return r.transitionToCompleting(ctx, exp, log)
	}

	if elapsed >= duration {
		log.Info("duration elapsed, transitioning to Completing")
		return r.transitionToCompleting(ctx, exp, log)
	}

	exec, err := r.Registry.Get(exp.Spec.Action.Type)
	if err != nil {
		return r.setFailed(ctx, exp, fmt.Sprintf("executor lost: %v", err))
	}

	r.Recorder.Event(exp, "Normal", "Executing", "Executing chaos action")
	if err := exec.Execute(ctx, exp); err != nil {
		log.Error("execute failed, attempting rollback", "error", err)
		r.Recorder.Event(exp, "Warning", "ExecuteFailed", fmt.Sprintf("Execution failed: %v", err))
		rbErr := exec.Rollback(ctx, exp)
		if rbErr != nil {
			r.Recorder.Event(exp, "Warning", "RollbackFailed", fmt.Sprintf("Rollback also failed: %v", rbErr))
			return r.setFailed(ctx, exp, fmt.Sprintf("execution failed: %v; rollback also failed: %v", err, rbErr))
		}
		return r.setFailed(ctx, exp, fmt.Sprintf("execution failed: %v", err))
	}

	remaining := duration - elapsed
	return ctrl.Result{RequeueAfter: remaining}, nil
}

func (r *ExperimentReconciler) handleAbort(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	exec, err := r.Registry.Get(exp.Spec.Action.Type)
	if err != nil {
		return r.setFailed(ctx, exp, fmt.Sprintf("abort: executor lost: %v", err))
	}

	r.Recorder.Event(exp, "Normal", "RollingBack", "Rolling back due to abort")
	if rbErr := exec.Rollback(ctx, exp); rbErr != nil {
		log.Error("rollback during abort failed", "error", rbErr)
		r.Recorder.Event(exp, "Warning", "RollbackFailed", fmt.Sprintf("Rollback failed during abort: %v", rbErr))
		return r.setFailed(ctx, exp, fmt.Sprintf("abort rollback failed: %v", rbErr))
	}

	return r.setAborted(ctx, exp)
}

func (r *ExperimentReconciler) transitionToCompleting(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	exp.Status.Phase = v1alpha1.PhaseCompleting
	setCondition(exp, "Progressing", metav1.ConditionTrue, "ExperimentCompleting", "Experiment is rolling back")

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("transitioning to Completing")
	return ctrl.Result{Requeue: true}, nil
}

func (r *ExperimentReconciler) reconcileCompleting(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	exec, err := r.Registry.Get(exp.Spec.Action.Type)
	if err != nil {
		return r.setFailed(ctx, exp, fmt.Sprintf("completing: executor lost: %v", err))
	}

	r.Recorder.Event(exp, "Normal", "RollingBack", "Rolling back chaos action")
	if rbErr := exec.Rollback(ctx, exp); rbErr != nil {
		log.Error("rollback failed", "error", rbErr)
		r.Recorder.Event(exp, "Warning", "RollbackFailed", fmt.Sprintf("Rollback failed: %v", rbErr))
		return r.setFailed(ctx, exp, fmt.Sprintf("rollback failed: %v", rbErr))
	}

	return r.setCompleted(ctx, exp)
}

func (r *ExperimentReconciler) handleDeletion(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(exp, finalizerName) {
		return ctrl.Result{}, nil
	}

	if exp.Status.Phase == v1alpha1.PhaseRunning || exp.Status.Phase == v1alpha1.PhaseCompleting {
		log.Info("handling deletion, running rollback")
		if exec, err := r.Registry.Get(exp.Spec.Action.Type); err == nil {
			r.Recorder.Event(exp, "Normal", "RollingBack", "Rolling back due to deletion")
			if rbErr := exec.Rollback(ctx, exp); rbErr != nil {
				log.Error("rollback during deletion failed", "error", rbErr)
				r.Recorder.Event(exp, "Warning", "RollbackFailed", fmt.Sprintf("Rollback during deletion failed: %v", rbErr))
			} else {
				r.Recorder.Event(exp, "Normal", "Completed", "Rollback completed during deletion")
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
	exp.Status.Message = reason
	setCondition(exp, "Progressing", metav1.ConditionFalse, "ExperimentFailed", reason)
	setCondition(exp, "Available", metav1.ConditionFalse, "ExperimentFailed", reason)
	setCondition(exp, "Degraded", metav1.ConditionTrue, "ExperimentFailed", reason)

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
	setCondition(exp, "Available", metav1.ConditionTrue, "ExperimentCompleted", "Experiment completed successfully")
	setCondition(exp, "Degraded", metav1.ConditionFalse, "ExperimentCompleted", "Experiment completed successfully")

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Event(exp, "Normal", "Completed", "Experiment completed successfully")
	return ctrl.Result{}, nil
}

func (r *ExperimentReconciler) setAborted(ctx context.Context, exp *v1alpha1.ChaosExperiment) (ctrl.Result, error) {
	now := metav1.Now()
	exp.Status.Phase = v1alpha1.PhaseAborted
	exp.Status.EndTime = &now
	setCondition(exp, "Progressing", metav1.ConditionFalse, "ExperimentAborted", "Experiment was aborted")
	setCondition(exp, "Available", metav1.ConditionFalse, "ExperimentAborted", "Experiment was aborted")
	setCondition(exp, "Degraded", metav1.ConditionFalse, "ExperimentAborted", "Experiment was aborted")

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Event(exp, "Warning", "Aborted", "Experiment was aborted")
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
