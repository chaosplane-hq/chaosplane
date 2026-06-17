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
	"github.com/chaosplane-hq/chaosplane/internal/probe"
)

const (
	finalizerName      = "chaosplane.dev/experiment-protection"
	abortAnnotation    = "chaosplane.dev/abort"
	executedAnnotation = "chaosplane.dev/executed"
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
	case v1alpha1.PhaseSteadyStateChecking:
		return r.reconcileSteadyStateChecking(ctx, &exp, log)
	case v1alpha1.PhaseRunning:
		return r.reconcileRunning(ctx, &exp, log)
	case v1alpha1.PhaseCompleting:
		return r.reconcileCompleting(ctx, &exp, log)
	case v1alpha1.PhaseRecovering:
		return r.reconcileRecovering(ctx, &exp, log)
	case v1alpha1.PhaseCompleted, v1alpha1.PhaseFailed, v1alpha1.PhaseAborted:
		return ctrl.Result{}, nil
	default:
		log.Warn("unknown phase", "phase", exp.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *ExperimentReconciler) reconcilePending(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	expRef := fmt.Sprintf("%s/%s", exp.Namespace, exp.Name)

	exec, err := r.Registry.Get(exp.Spec.Action.Type)
	if err != nil {
		log.Error("no executor found", "experiment", expRef, "action", exp.Spec.Action.Type)
		return r.setFailed(ctx, exp, fmt.Sprintf("experiment %s: no executor for action %q", expRef, exp.Spec.Action.Type))
	}

	if err := exec.Validate(exp); err != nil {
		log.Error("validation failed", "experiment", expRef, "error", err)
		return r.setFailed(ctx, exp, fmt.Sprintf("experiment %s: validation failed: %v", expRef, err))
	}

	if exp.Spec.SteadyState != nil && len(exp.Spec.SteadyState.Before) > 0 {
		exp.Status.Phase = v1alpha1.PhaseSteadyStateChecking
		exp.Status.ObservedGeneration = exp.Generation
		setCondition(exp, "Progressing", metav1.ConditionTrue, "SteadyStateChecking", "Running before probes")

		if err := r.Status().Update(ctx, exp); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(exp, "Normal", "SteadyStateChecking", "Experiment %s: running %d before steady-state probes", expRef, len(exp.Spec.SteadyState.Before))
		log.Info("transitioning to SteadyStateChecking", "experiment", expRef, "probeCount", len(exp.Spec.SteadyState.Before))
		return ctrl.Result{Requeue: true}, nil
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

	r.Recorder.Eventf(exp, "Normal", "Started", "Experiment %s started with action %q, duration %s", expRef, exp.Spec.Action.Type, exp.Spec.Duration.Duration)
	log.Info("transitioning to Running", "experiment", expRef, "action", exp.Spec.Action.Type, "duration", exp.Spec.Duration.Duration)
	return ctrl.Result{Requeue: true}, nil
}

func (r *ExperimentReconciler) reconcileSteadyStateChecking(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	expRef := fmt.Sprintf("%s/%s", exp.Namespace, exp.Name)

	if exp.Spec.SteadyState == nil {
		log.Error("steady-state spec missing", "experiment", expRef, "phase", "SteadyStateChecking")
		return r.setFailed(ctx, exp, fmt.Sprintf("experiment %s: steady-state spec missing in SteadyStateChecking phase", expRef))
	}

	r.Recorder.Eventf(exp, "Normal", "SteadyStateCheckStarted", "Experiment %s: evaluating %d before probes", expRef, len(exp.Spec.SteadyState.Before))
	ok, err := probe.RunAll(ctx, exp.Spec.SteadyState.Before, r.Client)
	if err != nil {
		log.Error("before probe error", "experiment", expRef, "error", err)
		r.Recorder.Eventf(exp, "Warning", "SteadyStateCheckFailed", "Experiment %s: before probe error: %v", expRef, err)
		return r.setFailed(ctx, exp, fmt.Sprintf("experiment %s: before probe failed: %v", expRef, err))
	}
	if !ok {
		log.Warn("before steady-state probe did not pass", "experiment", expRef)
		r.Recorder.Eventf(exp, "Warning", "SteadyStateCheckFailed", "Experiment %s: before steady-state probe did not pass", expRef)
		return r.setFailed(ctx, exp, fmt.Sprintf("experiment %s: before steady-state probe did not pass", expRef))
	}

	r.Recorder.Eventf(exp, "Normal", "SteadyStateCheckPassed", "Experiment %s: all before probes passed", expRef)

	now := metav1.Now()
	exp.Status.Phase = v1alpha1.PhaseRunning
	exp.Status.StartTime = &now
	setCondition(exp, "Progressing", metav1.ConditionTrue, "ExperimentStarted", "Before probes passed, experiment is running")
	setCondition(exp, "Available", metav1.ConditionFalse, "ExperimentStarted", "Experiment is running")
	setCondition(exp, "Degraded", metav1.ConditionFalse, "ExperimentStarted", "Experiment is running")

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Eventf(exp, "Normal", "Started", "Experiment %s: before probes passed, experiment started with action %q", expRef, exp.Spec.Action.Type)
	log.Info("before probes passed, transitioning to Running", "experiment", expRef)
	return ctrl.Result{Requeue: true}, nil
}

func (r *ExperimentReconciler) reconcileRunning(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	expRef := fmt.Sprintf("%s/%s", exp.Namespace, exp.Name)

	if exp.Annotations[abortAnnotation] == "true" {
		log.Info("abort annotation detected, rolling back", "experiment", expRef)
		r.Recorder.Eventf(exp, "Warning", "AbortTriggered", "Experiment %s: abort annotation detected, initiating rollback", expRef)
		return r.handleAbort(ctx, exp, log)
	}

	if len(exp.Spec.AbortConditions) > 0 {
		triggered, action, err := r.checkAbortConditions(ctx, exp, log)
		if err != nil {
			log.Error("abort condition check failed", "experiment", expRef, "error", err)
		} else if triggered {
			switch action {
			case v1alpha1.AbortActionRollback:
				log.Info("abort condition triggered, rolling back", "experiment", expRef)
				r.Recorder.Eventf(exp, "Warning", "AbortConditionTriggered", "Experiment %s: abort condition met, rolling back", expRef)
				return r.transitionToCompleting(ctx, exp, log)
			default:
				log.Info("abort condition triggered, aborting", "experiment", expRef)
				r.Recorder.Eventf(exp, "Warning", "AbortConditionTriggered", "Experiment %s: abort condition met, aborting", expRef)
				return r.handleAbort(ctx, exp, log)
			}
		}
	}

	if exp.Status.StartTime == nil {
		log.Error("running experiment has no startTime", "experiment", expRef)
		return r.setFailed(ctx, exp, fmt.Sprintf("experiment %s: running experiment has no startTime", expRef))
	}

	elapsed := time.Since(exp.Status.StartTime.Time)
	duration := exp.Spec.Duration.Duration
	timeout := duration * 2

	if elapsed >= timeout {
		log.Warn("experiment timed out", "experiment", expRef, "elapsed", elapsed, "timeout", timeout)
		r.Recorder.Eventf(exp, "Warning", "Timeout", "Experiment %s: timed out after %s (timeout: %s), forcing rollback", expRef, elapsed.Round(time.Second), timeout)
		return r.transitionToCompleting(ctx, exp, log)
	}

	if elapsed >= duration {
		log.Info("duration elapsed, transitioning to Completing", "experiment", expRef, "elapsed", elapsed, "duration", duration)
		r.Recorder.Eventf(exp, "Normal", "DurationElapsed", "Experiment %s: duration %s elapsed, transitioning to rollback", expRef, duration)
		return r.transitionToCompleting(ctx, exp, log)
	}

	// Only execute once per experiment to prevent non-idempotent actions from repeating
	if exp.Annotations[executedAnnotation] != "true" {
		exec, err := r.Registry.Get(exp.Spec.Action.Type)
		if err != nil {
			log.Error("executor lost during running phase", "experiment", expRef, "action", exp.Spec.Action.Type)
			return r.setFailed(ctx, exp, fmt.Sprintf("experiment %s: executor for action %q lost during running phase: %v", expRef, exp.Spec.Action.Type, err))
		}

		r.Recorder.Eventf(exp, "Normal", "Executing", "Experiment %s: executing action %q", expRef, exp.Spec.Action.Type)
		if err := exec.Execute(ctx, exp); err != nil {
			log.Error("execute failed, attempting rollback", "experiment", expRef, "action", exp.Spec.Action.Type, "error", err)
			r.Recorder.Eventf(exp, "Warning", "ExecuteFailed", "Experiment %s: action %q failed: %v", expRef, exp.Spec.Action.Type, err)
			r.Recorder.Eventf(exp, "Normal", "RollbackStarted", "Experiment %s: initiating rollback after execution failure", expRef)
			rbErr := exec.Rollback(ctx, exp)
			if rbErr != nil {
				log.Error("rollback also failed", "experiment", expRef, "executeError", err, "rollbackError", rbErr)
				r.Recorder.Eventf(exp, "Warning", "RollbackFailed", "Experiment %s: rollback also failed: %v", expRef, rbErr)
				return r.setFailed(ctx, exp, fmt.Sprintf("experiment %s: execution failed: %v; rollback also failed: %v", expRef, err, rbErr))
			}
			r.Recorder.Eventf(exp, "Normal", "RollbackCompleted", "Experiment %s: rollback completed after execution failure", expRef)
			return r.setFailed(ctx, exp, fmt.Sprintf("experiment %s: execution failed: %v", expRef, err))
		}

		// Mark as executed so subsequent reconciles don't re-run the action
		patch := client.MergeFrom(exp.DeepCopy())
		if exp.Annotations == nil {
			exp.Annotations = map[string]string{}
		}
		exp.Annotations[executedAnnotation] = "true"
		if err := r.Patch(ctx, exp, patch); err != nil {
			log.Error("failed to mark executed annotation", "experiment", expRef, "error", err)
		}
	}

	remaining := duration - elapsed
	if len(exp.Spec.AbortConditions) > 0 && remaining > 5*time.Second {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return ctrl.Result{RequeueAfter: remaining}, nil
}

func abortConditionToProbeSpec(ac v1alpha1.AbortConditionSpec) v1alpha1.ProbeSpec {
	return v1alpha1.ProbeSpec{
		Name:       ac.Name,
		Type:       ac.Type,
		Prometheus: ac.Prometheus,
		HTTP:       ac.HTTP,
		K8s:        ac.K8s,
	}
}

func (r *ExperimentReconciler) checkAbortConditions(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (bool, v1alpha1.AbortAction, error) {
	for _, ac := range exp.Spec.AbortConditions {
		spec := abortConditionToProbeSpec(ac)
		p, err := probe.NewProbe(spec, r.Client)
		if err != nil {
			return false, "", fmt.Errorf("abort condition %q: %w", ac.Name, err)
		}
		ok, err := p.Run(ctx)
		if err != nil {
			return false, "", fmt.Errorf("abort condition %q: %w", ac.Name, err)
		}
		if ok {
			log.Info("abort condition triggered", "condition", ac.Name, "action", ac.Action)
			return true, ac.Action, nil
		}
	}
	return false, "", nil
}

func (r *ExperimentReconciler) handleAbort(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	expRef := fmt.Sprintf("%s/%s", exp.Namespace, exp.Name)

	exec, err := r.Registry.Get(exp.Spec.Action.Type)
	if err != nil {
		log.Error("executor lost during abort", "experiment", expRef, "action", exp.Spec.Action.Type)
		return r.setFailed(ctx, exp, fmt.Sprintf("experiment %s: abort: executor for action %q lost: %v", expRef, exp.Spec.Action.Type, err))
	}

	r.Recorder.Eventf(exp, "Normal", "RollbackStarted", "Experiment %s: rolling back due to abort", expRef)
	if rbErr := exec.Rollback(ctx, exp); rbErr != nil {
		log.Error("rollback during abort failed", "experiment", expRef, "error", rbErr)
		r.Recorder.Eventf(exp, "Warning", "RollbackFailed", "Experiment %s: rollback failed during abort: %v", expRef, rbErr)
		return r.setFailed(ctx, exp, fmt.Sprintf("experiment %s: abort rollback failed: %v", expRef, rbErr))
	}

	r.Recorder.Eventf(exp, "Normal", "RollbackCompleted", "Experiment %s: rollback completed during abort", expRef)
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

	if exp.Spec.SteadyState != nil && len(exp.Spec.SteadyState.After) > 0 {
		now := metav1.Now()
		exp.Status.Phase = v1alpha1.PhaseRecovering
		exp.Status.RecoveryStartTime = &now
		setCondition(exp, "Progressing", metav1.ConditionTrue, "Recovering", "Running after probes")

		if err := r.Status().Update(ctx, exp); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(exp, "Normal", "Recovering", "Running after steady-state probes")
		log.Info("transitioning to Recovering")
		return ctrl.Result{Requeue: true}, nil
	}

	return r.setCompleted(ctx, exp)
}

func (r *ExperimentReconciler) reconcileRecovering(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	if exp.Spec.SteadyState == nil {
		return r.setFailed(ctx, exp, "steady-state spec missing in Recovering phase")
	}

	if exp.Status.RecoveryStartTime != nil && exp.Spec.SteadyState.RecoveryTimeout.Duration > 0 {
		elapsed := time.Since(exp.Status.RecoveryStartTime.Time)
		if elapsed >= exp.Spec.SteadyState.RecoveryTimeout.Duration {
			log.Warn("recovery timeout exceeded", "elapsed", elapsed)
			return r.setFailed(ctx, exp, "after steady-state probes did not pass within recovery timeout")
		}
	}

	ok, err := probe.RunAll(ctx, exp.Spec.SteadyState.After, r.Client)
	if err != nil {
		log.Error("after probe error", "error", err)
		return r.setFailed(ctx, exp, fmt.Sprintf("after probe failed: %v", err))
	}
	if !ok {
		log.Info("after probes not yet passing, requeueing")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	r.Recorder.Event(exp, "Normal", "Recovered", "After probes passed")
	log.Info("after probes passed, transitioning to Completed")
	return r.setCompleted(ctx, exp)
}

func (r *ExperimentReconciler) handleDeletion(ctx context.Context, exp *v1alpha1.ChaosExperiment, log *slog.Logger) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(exp, finalizerName) {
		return ctrl.Result{}, nil
	}

	if exp.Status.Phase == v1alpha1.PhaseRunning || exp.Status.Phase == v1alpha1.PhaseCompleting || exp.Status.Phase == v1alpha1.PhaseRecovering {
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
