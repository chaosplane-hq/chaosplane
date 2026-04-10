package controller

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

const (
	workflowFinalizerName = "chaosplane.io/workflow-protection"
	workflowAbortAnnotation = "chaosplane.io/abort"
	workflowResumeAnnotation = "chaosplane.io/resume"
	maxRetries = 3
)

type WorkflowReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Logger   *slog.Logger
}

func (r *WorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Logger.With("workflow", req.NamespacedName)

	var wf v1alpha1.ChaosWorkflow
	if err := r.Get(ctx, req.NamespacedName, &wf); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if wf.Annotations[workflowAbortAnnotation] == "true" && wf.Status.Phase == v1alpha1.WorkflowPhaseRunning {
		return r.handleAbort(ctx, &wf, log)
	}

	switch wf.Status.Phase {
	case "", v1alpha1.WorkflowPhasePending:
		return r.reconcilePending(ctx, &wf, log)
	case v1alpha1.WorkflowPhaseRunning:
		return r.reconcileRunning(ctx, &wf, log)
	case v1alpha1.WorkflowPhasePaused:
		return r.reconcilePaused(ctx, &wf, log)
	case v1alpha1.WorkflowPhaseCompleted, v1alpha1.WorkflowPhaseFailed, v1alpha1.WorkflowPhaseAborted:
		return ctrl.Result{}, nil
	default:
		log.Warn("unknown phase", "phase", wf.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// --- DAG Engine ---

type dagNode struct {
	template  v1alpha1.WorkflowTemplate
	inDegree  int
	dependents []string
}

func buildDAG(templates []v1alpha1.WorkflowTemplate) (map[string]*dagNode, error) {
	nodes := make(map[string]*dagNode, len(templates))
	for _, t := range templates {
		nodes[t.Name] = &dagNode{template: t}
	}

	for _, t := range templates {
		for _, dep := range t.Dependencies {
			parent, ok := nodes[dep]
			if !ok {
				return nil, fmt.Errorf("template %q depends on unknown template %q", t.Name, dep)
			}
			parent.dependents = append(parent.dependents, t.Name)
			nodes[t.Name].inDegree++
		}
	}
	return nodes, nil
}

func TopologicalSort(templates []v1alpha1.WorkflowTemplate) ([]string, error) {
	nodes, err := buildDAG(templates)
	if err != nil {
		return nil, err
	}

	var queue []string
	for name, node := range nodes {
		if node.inDegree == 0 {
			queue = append(queue, name)
		}
	}

	var order []string
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		order = append(order, name)

		for _, dep := range nodes[name].dependents {
			nodes[dep].inDegree--
			if nodes[dep].inDegree == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(templates) {
		return nil, fmt.Errorf("cycle detected in workflow DAG")
	}
	return order, nil
}

func ReadyTemplates(templates []v1alpha1.WorkflowTemplate, completed map[string]bool) []string {
	nodes, err := buildDAG(templates)
	if err != nil {
		return nil
	}

	var ready []string
	for name, node := range nodes {
		if completed[name] {
			continue
		}
		allDepsMet := true
		for _, dep := range node.template.Dependencies {
			if !completed[dep] {
				allDepsMet = false
				break
			}
		}
		if allDepsMet {
			ready = append(ready, name)
		}
	}
	return ready
}

func FormatProgress(statuses []v1alpha1.TemplateStatus, total int) string {
	completed := 0
	for _, s := range statuses {
		if s.Phase == v1alpha1.WorkflowPhaseCompleted || s.Phase == v1alpha1.WorkflowPhaseFailed {
			completed++
		}
	}
	return fmt.Sprintf("%d/%d", completed, total)
}

// --- Reconcile phases ---

func (r *WorkflowReconciler) reconcilePending(ctx context.Context, wf *v1alpha1.ChaosWorkflow, log *slog.Logger) (ctrl.Result, error) {
	if _, err := TopologicalSort(wf.Spec.Templates); err != nil {
		return r.setFailed(ctx, wf, err.Error())
	}

	now := metav1.Now()
	wf.Status.Phase = v1alpha1.WorkflowPhaseRunning
	wf.Status.StartTime = &now
	wf.Status.ObservedGeneration = wf.Generation
	wf.Status.Progress = FormatProgress(nil, len(wf.Spec.Templates))
	setWorkflowCondition(wf, "Progressing", metav1.ConditionTrue, "WorkflowStarted", "Workflow is running")

	if err := r.Status().Update(ctx, wf); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Event(wf, "Normal", "Started", "Workflow started")
	log.Info("workflow started")
	return ctrl.Result{Requeue: true}, nil
}

func (r *WorkflowReconciler) reconcileRunning(ctx context.Context, wf *v1alpha1.ChaosWorkflow, log *slog.Logger) (ctrl.Result, error) {
	statusMap := make(map[string]*v1alpha1.TemplateStatus)
	for i := range wf.Status.TemplateStatuses {
		statusMap[wf.Status.TemplateStatuses[i].Name] = &wf.Status.TemplateStatuses[i]
	}

	completed := make(map[string]bool)
	running := make(map[string]bool)
	failed := make(map[string]bool)
	for name, s := range statusMap {
		switch s.Phase {
		case v1alpha1.WorkflowPhaseCompleted:
			completed[name] = true
		case v1alpha1.WorkflowPhaseRunning:
			running[name] = true
		case v1alpha1.WorkflowPhaseFailed:
			failed[name] = true
			completed[name] = true
		}
	}

	if len(failed) > 0 {
		strategy := wf.Spec.ErrorHandling.Strategy
		if strategy == "" {
			strategy = v1alpha1.ErrorStrategyAbort
		}
		switch strategy {
		case v1alpha1.ErrorStrategyAbort, v1alpha1.ErrorStrategyRollback:
			return r.setFailed(ctx, wf, "template failed, strategy: "+string(strategy))
		case v1alpha1.ErrorStrategyContinue:
			// continue processing
		case v1alpha1.ErrorStrategyRetry:
			for name := range failed {
				ts := statusMap[name]
				retryCount := 0
				if ts.Message != "" {
					fmt.Sscanf(ts.Message, "retry %d", &retryCount)
				}
				if retryCount < maxRetries {
					ts.Phase = v1alpha1.WorkflowPhasePending
					ts.Message = fmt.Sprintf("retry %d", retryCount+1)
					delete(completed, name)
					delete(failed, name)
				}
			}
		}
	}

	if len(completed) == len(wf.Spec.Templates) {
		return r.setCompleted(ctx, wf)
	}

	ready := ReadyTemplates(wf.Spec.Templates, completed)
	requeueAfter := time.Duration(0)

	for _, name := range ready {
		if running[name] {
			continue
		}
		tmpl := findTemplate(wf.Spec.Templates, name)
		if tmpl == nil {
			continue
		}

		result, err := r.executeTemplate(ctx, wf, tmpl, statusMap, log)
		if err != nil {
			return ctrl.Result{}, err
		}
		if result.RequeueAfter > 0 && (requeueAfter == 0 || result.RequeueAfter < requeueAfter) {
			requeueAfter = result.RequeueAfter
		}
	}

	for name := range running {
		tmpl := findTemplate(wf.Spec.Templates, name)
		if tmpl == nil {
			continue
		}
		result, err := r.checkTemplateProgress(ctx, wf, tmpl, statusMap, log)
		if err != nil {
			return ctrl.Result{}, err
		}
		if result.RequeueAfter > 0 && (requeueAfter == 0 || result.RequeueAfter < requeueAfter) {
			requeueAfter = result.RequeueAfter
		}
	}

	wf.Status.TemplateStatuses = flattenStatuses(statusMap)
	wf.Status.Progress = FormatProgress(wf.Status.TemplateStatuses, len(wf.Spec.Templates))

	if err := r.Status().Update(ctx, wf); err != nil {
		return ctrl.Result{}, err
	}

	if requeueAfter > 0 {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *WorkflowReconciler) reconcilePaused(ctx context.Context, wf *v1alpha1.ChaosWorkflow, log *slog.Logger) (ctrl.Result, error) {
	if wf.Annotations[workflowResumeAnnotation] == "true" {
		wf.Status.Phase = v1alpha1.WorkflowPhaseRunning
		setWorkflowCondition(wf, "Progressing", metav1.ConditionTrue, "WorkflowResumed", "Workflow resumed")
		if err := r.Status().Update(ctx, wf); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(wf, "Normal", "Resumed", "Workflow resumed")
		log.Info("workflow resumed")
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// --- Template execution ---

func (r *WorkflowReconciler) executeTemplate(ctx context.Context, wf *v1alpha1.ChaosWorkflow, tmpl *v1alpha1.WorkflowTemplate, statusMap map[string]*v1alpha1.TemplateStatus, log *slog.Logger) (ctrl.Result, error) {
	now := metav1.Now()
	if statusMap[tmpl.Name] == nil {
		statusMap[tmpl.Name] = &v1alpha1.TemplateStatus{Name: tmpl.Name}
	}
	ts := statusMap[tmpl.Name]

	switch tmpl.Type {
	case v1alpha1.TemplateTypeExperiment:
		ts.Phase = v1alpha1.WorkflowPhaseRunning
		ts.StartTime = &now
		ts.Message = "creating experiment"

		if tmpl.ExperimentRef == nil {
			ts.Phase = v1alpha1.WorkflowPhaseFailed
			ts.Message = "experimentRef is required"
			return ctrl.Result{}, nil
		}

		ns := tmpl.ExperimentRef.Namespace
		if ns == "" {
			ns = wf.Namespace
		}

		exp := &v1alpha1.ChaosExperiment{}
		err := r.Get(ctx, types.NamespacedName{Name: tmpl.ExperimentRef.Name, Namespace: ns}, exp)
		if err != nil {
			if client.IgnoreNotFound(err) == nil {
				ts.Message = "waiting for experiment to be created"
			} else {
				ts.Phase = v1alpha1.WorkflowPhaseFailed
				ts.Message = fmt.Sprintf("failed to get experiment: %v", err)
			}
		}
		r.Recorder.Event(wf, "Normal", "TemplateStarted", fmt.Sprintf("Started experiment template %q", tmpl.Name))
		log.Info("started experiment template", "template", tmpl.Name)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil

	case v1alpha1.TemplateTypeDelay:
		ts.Phase = v1alpha1.WorkflowPhaseRunning
		ts.StartTime = &now
		if tmpl.Delay == nil {
			ts.Phase = v1alpha1.WorkflowPhaseFailed
			ts.Message = "delay spec is required"
			return ctrl.Result{}, nil
		}
		ts.Message = fmt.Sprintf("waiting %s", tmpl.Delay.Duration.Duration)
		r.Recorder.Event(wf, "Normal", "TemplateStarted", fmt.Sprintf("Started delay template %q for %s", tmpl.Name, tmpl.Delay.Duration.Duration))
		log.Info("started delay template", "template", tmpl.Name, "duration", tmpl.Delay.Duration.Duration)
		return ctrl.Result{RequeueAfter: tmpl.Delay.Duration.Duration}, nil

	case v1alpha1.TemplateTypeCondition:
		ts.Phase = v1alpha1.WorkflowPhaseRunning
		ts.StartTime = &now
		if tmpl.Condition == nil {
			ts.Phase = v1alpha1.WorkflowPhaseFailed
			ts.Message = "condition spec is required"
			return ctrl.Result{}, nil
		}

		result := evaluateCondition(tmpl.Condition.Expression)
		endNow := metav1.Now()
		ts.Phase = v1alpha1.WorkflowPhaseCompleted
		ts.EndTime = &endNow

		if result {
			ts.Message = fmt.Sprintf("condition true, routing to %s", tmpl.Condition.IfTrue)
		} else {
			ts.Message = fmt.Sprintf("condition false, routing to %s", tmpl.Condition.IfFalse)
		}
		r.Recorder.Event(wf, "Normal", "ConditionEvaluated", ts.Message)
		log.Info("condition evaluated", "template", tmpl.Name, "result", result)
		return ctrl.Result{Requeue: true}, nil

	case v1alpha1.TemplateTypeParallel:
		ts.Phase = v1alpha1.WorkflowPhaseRunning
		ts.StartTime = &now
		if tmpl.Parallel == nil {
			ts.Phase = v1alpha1.WorkflowPhaseFailed
			ts.Message = "parallel spec is required"
			return ctrl.Result{}, nil
		}
		ts.Message = fmt.Sprintf("running %d parallel templates", len(tmpl.Parallel.Templates))
		r.Recorder.Event(wf, "Normal", "TemplateStarted", fmt.Sprintf("Started parallel template %q with %d sub-templates", tmpl.Name, len(tmpl.Parallel.Templates)))
		log.Info("started parallel template", "template", tmpl.Name)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil

	case v1alpha1.TemplateTypeSuspend:
		ts.Phase = v1alpha1.WorkflowPhaseRunning
		ts.StartTime = &now
		ts.Message = "waiting for resume"
		r.Recorder.Event(wf, "Normal", "TemplateStarted", fmt.Sprintf("Suspended at template %q", tmpl.Name))
		log.Info("suspended at template", "template", tmpl.Name)

		wf.Status.Phase = v1alpha1.WorkflowPhasePaused
		setWorkflowCondition(wf, "Progressing", metav1.ConditionFalse, "WorkflowPaused", "Waiting for resume at "+tmpl.Name)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	default:
		ts.Phase = v1alpha1.WorkflowPhaseFailed
		ts.Message = fmt.Sprintf("unknown template type: %s", tmpl.Type)
		return ctrl.Result{}, nil
	}
}

func (r *WorkflowReconciler) checkTemplateProgress(ctx context.Context, wf *v1alpha1.ChaosWorkflow, tmpl *v1alpha1.WorkflowTemplate, statusMap map[string]*v1alpha1.TemplateStatus, log *slog.Logger) (ctrl.Result, error) {
	ts := statusMap[tmpl.Name]
	if ts == nil {
		return ctrl.Result{}, nil
	}

	switch tmpl.Type {
	case v1alpha1.TemplateTypeExperiment:
		if tmpl.ExperimentRef == nil {
			return ctrl.Result{}, nil
		}
		ns := tmpl.ExperimentRef.Namespace
		if ns == "" {
			ns = wf.Namespace
		}
		exp := &v1alpha1.ChaosExperiment{}
		err := r.Get(ctx, types.NamespacedName{Name: tmpl.ExperimentRef.Name, Namespace: ns}, exp)
		if err != nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		switch exp.Status.Phase {
		case v1alpha1.PhaseCompleted:
			now := metav1.Now()
			ts.Phase = v1alpha1.WorkflowPhaseCompleted
			ts.EndTime = &now
			ts.Message = "experiment completed"
			log.Info("experiment template completed", "template", tmpl.Name)
		case v1alpha1.PhaseFailed, v1alpha1.PhaseAborted:
			now := metav1.Now()
			ts.Phase = v1alpha1.WorkflowPhaseFailed
			ts.EndTime = &now
			ts.Message = fmt.Sprintf("experiment %s", exp.Status.Phase)
			log.Info("experiment template failed", "template", tmpl.Name, "phase", exp.Status.Phase)
		default:
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

	case v1alpha1.TemplateTypeDelay:
		if ts.StartTime != nil && tmpl.Delay != nil {
			elapsed := time.Since(ts.StartTime.Time)
			if elapsed >= tmpl.Delay.Duration.Duration {
				now := metav1.Now()
				ts.Phase = v1alpha1.WorkflowPhaseCompleted
				ts.EndTime = &now
				ts.Message = "delay completed"
				log.Info("delay template completed", "template", tmpl.Name)
			} else {
				remaining := tmpl.Delay.Duration.Duration - elapsed
				return ctrl.Result{RequeueAfter: remaining}, nil
			}
		}

	case v1alpha1.TemplateTypeParallel:
		if tmpl.Parallel == nil {
			return ctrl.Result{}, nil
		}
		allDone := true
		for _, subName := range tmpl.Parallel.Templates {
			subStatus := statusMap[subName]
			if subStatus == nil || (subStatus.Phase != v1alpha1.WorkflowPhaseCompleted && subStatus.Phase != v1alpha1.WorkflowPhaseFailed) {
				allDone = false
				break
			}
		}
		if allDone {
			now := metav1.Now()
			ts.Phase = v1alpha1.WorkflowPhaseCompleted
			ts.EndTime = &now
			ts.Message = "all parallel templates completed"
			log.Info("parallel template completed", "template", tmpl.Name)
		} else {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

	case v1alpha1.TemplateTypeSuspend:
		if wf.Annotations[workflowResumeAnnotation] == "true" {
			now := metav1.Now()
			ts.Phase = v1alpha1.WorkflowPhaseCompleted
			ts.EndTime = &now
			ts.Message = "resumed"
			log.Info("suspend template resumed", "template", tmpl.Name)
		} else if tmpl.Suspend != nil && tmpl.Suspend.Timeout != nil && ts.StartTime != nil {
			elapsed := time.Since(ts.StartTime.Time)
			if elapsed >= tmpl.Suspend.Timeout.Duration {
				now := metav1.Now()
				ts.Phase = v1alpha1.WorkflowPhaseCompleted
				ts.EndTime = &now
				ts.Message = "suspend timed out"
				log.Info("suspend template timed out", "template", tmpl.Name)
			}
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// --- Abort ---

func (r *WorkflowReconciler) handleAbort(ctx context.Context, wf *v1alpha1.ChaosWorkflow, log *slog.Logger) (ctrl.Result, error) {
	log.Info("abort annotation detected, aborting workflow")

	var expList v1alpha1.ChaosExperimentList
	if err := r.List(ctx, &expList, client.InNamespace(wf.Namespace)); err != nil {
		log.Error("failed to list experiments for abort", "error", err)
	} else {
		for i := range expList.Items {
			exp := &expList.Items[i]
			if isOwnedBy(exp, wf) && (exp.Status.Phase == v1alpha1.PhaseRunning || exp.Status.Phase == v1alpha1.PhasePending) {
				if exp.Annotations == nil {
					exp.Annotations = make(map[string]string)
				}
				exp.Annotations[abortAnnotation] = "true"
				if err := r.Update(ctx, exp); err != nil {
					log.Error("failed to abort experiment", "experiment", exp.Name, "error", err)
				} else {
					log.Info("aborted experiment", "experiment", exp.Name)
				}
			}
		}
	}

	return r.setAborted(ctx, wf)
}

func isOwnedBy(exp *v1alpha1.ChaosExperiment, wf *v1alpha1.ChaosWorkflow) bool {
	for _, ref := range exp.OwnerReferences {
		if ref.UID == wf.UID {
			return true
		}
	}
	return false
}

// --- Terminal states ---

func (r *WorkflowReconciler) setFailed(ctx context.Context, wf *v1alpha1.ChaosWorkflow, reason string) (ctrl.Result, error) {
	now := metav1.Now()
	wf.Status.Phase = v1alpha1.WorkflowPhaseFailed
	wf.Status.EndTime = &now
	setWorkflowCondition(wf, "Progressing", metav1.ConditionFalse, "WorkflowFailed", reason)
	setWorkflowCondition(wf, "Available", metav1.ConditionFalse, "WorkflowFailed", reason)
	setWorkflowCondition(wf, "Degraded", metav1.ConditionTrue, "WorkflowFailed", reason)

	if err := r.Status().Update(ctx, wf); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Event(wf, "Warning", "Failed", reason)
	return ctrl.Result{}, nil
}

func (r *WorkflowReconciler) setCompleted(ctx context.Context, wf *v1alpha1.ChaosWorkflow) (ctrl.Result, error) {
	now := metav1.Now()
	wf.Status.Phase = v1alpha1.WorkflowPhaseCompleted
	wf.Status.EndTime = &now
	wf.Status.Progress = FormatProgress(wf.Status.TemplateStatuses, len(wf.Spec.Templates))
	setWorkflowCondition(wf, "Progressing", metav1.ConditionFalse, "WorkflowCompleted", "Workflow completed successfully")
	setWorkflowCondition(wf, "Available", metav1.ConditionTrue, "WorkflowCompleted", "Workflow completed successfully")
	setWorkflowCondition(wf, "Degraded", metav1.ConditionFalse, "WorkflowCompleted", "Workflow completed successfully")

	if err := r.Status().Update(ctx, wf); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Event(wf, "Normal", "Completed", "Workflow completed successfully")
	return ctrl.Result{}, nil
}

func (r *WorkflowReconciler) setAborted(ctx context.Context, wf *v1alpha1.ChaosWorkflow) (ctrl.Result, error) {
	now := metav1.Now()
	wf.Status.Phase = v1alpha1.WorkflowPhaseAborted
	wf.Status.EndTime = &now
	setWorkflowCondition(wf, "Progressing", metav1.ConditionFalse, "WorkflowAborted", "Workflow was aborted")
	setWorkflowCondition(wf, "Available", metav1.ConditionFalse, "WorkflowAborted", "Workflow was aborted")
	setWorkflowCondition(wf, "Degraded", metav1.ConditionFalse, "WorkflowAborted", "Workflow was aborted")

	if err := r.Status().Update(ctx, wf); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Event(wf, "Warning", "Aborted", "Workflow was aborted")
	return ctrl.Result{}, nil
}

// --- Helpers ---

func findTemplate(templates []v1alpha1.WorkflowTemplate, name string) *v1alpha1.WorkflowTemplate {
	for i := range templates {
		if templates[i].Name == name {
			return &templates[i]
		}
	}
	return nil
}

func flattenStatuses(statusMap map[string]*v1alpha1.TemplateStatus) []v1alpha1.TemplateStatus {
	result := make([]v1alpha1.TemplateStatus, 0, len(statusMap))
	for _, s := range statusMap {
		result = append(result, *s)
	}
	return result
}

func evaluateCondition(expression string) bool {
	expression = strings.TrimSpace(expression)
	if expression == "true" {
		return true
	}
	if expression == "false" {
		return false
	}
	parts := strings.SplitN(expression, "==", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]) == strings.TrimSpace(parts[1])
	}
	parts = strings.SplitN(expression, "!=", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]) != strings.TrimSpace(parts[1])
	}
	return false
}

func setWorkflowCondition(wf *v1alpha1.ChaosWorkflow, condType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	for i, c := range wf.Status.Conditions {
		if c.Type == condType {
			wf.Status.Conditions[i].Status = status
			wf.Status.Conditions[i].Reason = reason
			wf.Status.Conditions[i].Message = message
			wf.Status.Conditions[i].LastTransitionTime = now
			wf.Status.Conditions[i].ObservedGeneration = wf.Generation
			return
		}
	}
	wf.Status.Conditions = append(wf.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
		ObservedGeneration: wf.Generation,
	})
}

func (r *WorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ChaosWorkflow{}).
		Complete(r)
}
