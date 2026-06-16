package controller_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/controller"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

type fakeRecorder struct {
	events []string
}

func (f *fakeRecorder) Event(obj runtime.Object, eventtype, reason, message string) {
	f.events = append(f.events, fmt.Sprintf("%s %s %s", eventtype, reason, message))
}
func (f *fakeRecorder) Eventf(obj runtime.Object, eventtype, reason, messageFmt string, args ...any) {
	f.events = append(f.events, fmt.Sprintf("%s %s %s", eventtype, reason, fmt.Sprintf(messageFmt, args...)))
}
func (f *fakeRecorder) AnnotatedEventf(obj runtime.Object, _ map[string]string, eventtype, reason, messageFmt string, args ...any) {
	f.events = append(f.events, fmt.Sprintf("%s %s %s", eventtype, reason, fmt.Sprintf(messageFmt, args...)))
}

func newReconciler() (*controller.ExperimentReconciler, *fakeRecorder) {
	registry := executor.NewRegistry()
	registry.Register("pod-kill", pod.NewKillExecutor(slog.Default(), k8sClient, fake.NewSimpleClientset(targetPod())))
	rec := &fakeRecorder{}
	return &controller.ExperimentReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		Recorder: rec,
		Registry: registry,
		Logger:   slog.Default(),
	}, rec
}

// targetPod matches the label selector set by makeExperiment so the pod-kill
// executor can resolve and delete a target during reconciliation.
func targetPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "target-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main", Image: "busybox"}},
		},
	}
}

func makeExperiment(name string, duration time.Duration) *v1alpha1.ChaosExperiment {
	return &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-" + time.Now().Format("150405.000"),
			Namespace: "default",
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Kind:          "Pod",
				Namespace:     "default",
				LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			},
			Action:   v1alpha1.ActionSpec{Type: "pod-kill"},
			Duration: metav1.Duration{Duration: duration},
		},
	}
}

func reconcileN(ctx context.Context, r *controller.ExperimentReconciler, nn types.NamespacedName, n int) (ctrl.Result, error) {
	var res ctrl.Result
	var err error
	for range n {
		res, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

func getExperiment(ctx context.Context, nn types.NamespacedName) (*v1alpha1.ChaosExperiment, error) {
	var exp v1alpha1.ChaosExperiment
	err := k8sClient.Get(ctx, nn, &exp)
	return &exp, err
}

func hasCondition(exp *v1alpha1.ChaosExperiment, condType string, status metav1.ConditionStatus) bool {
	for _, c := range exp.Status.Conditions {
		if c.Type == condType && c.Status == status {
			return true
		}
	}
	return false
}

func hasEvent(rec *fakeRecorder, substr string) bool {
	for _, e := range rec.events {
		if len(e) >= len(substr) {
			for i := 0; i <= len(e)-len(substr); i++ {
				if e[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

func TestReconcilePendingToRunning(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	exp := makeExperiment("test-pending", 30*time.Second)
	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r, rec := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile 1: %v", err)
	}

	updated, err := getExperiment(ctx, nn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	hasFinalizer := false
	for _, f := range updated.Finalizers {
		if f == "chaosplane.dev/experiment-protection" {
			hasFinalizer = true
		}
	}
	if !hasFinalizer {
		t.Fatal("expected finalizer")
	}

	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile 2: %v", err)
	}

	updated, err = getExperiment(ctx, nn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseRunning {
		t.Fatalf("expected Running, got %s", updated.Status.Phase)
	}
	if updated.Status.StartTime == nil {
		t.Fatal("expected startTime")
	}
	if !hasCondition(updated, "Progressing", metav1.ConditionTrue) {
		t.Fatal("expected Progressing=True")
	}
	if !hasEvent(rec, "Started") {
		t.Fatal("expected Started event")
	}
}

func TestReconcileRunningToCompletingToCompleted(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	exp := makeExperiment("test-complete", 1*time.Millisecond)
	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r, rec := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := reconcileN(ctx, r, nn, 2); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile running: %v", err)
	}

	updated, err := getExperiment(ctx, nn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseCompleting {
		t.Fatalf("expected Completing, got %s", updated.Status.Phase)
	}

	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile completing: %v", err)
	}

	updated, err = getExperiment(ctx, nn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseCompleted {
		t.Fatalf("expected Completed, got %s", updated.Status.Phase)
	}
	if updated.Status.EndTime == nil {
		t.Fatal("expected endTime")
	}
	if !hasCondition(updated, "Available", metav1.ConditionTrue) {
		t.Fatal("expected Available=True")
	}
	if !hasCondition(updated, "Progressing", metav1.ConditionFalse) {
		t.Fatal("expected Progressing=False")
	}
	if !hasEvent(rec, "Completed") {
		t.Fatal("expected Completed event")
	}
}

func TestReconcileAbort(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	exp := makeExperiment("test-abort", 5*time.Minute)
	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r, rec := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := reconcileN(ctx, r, nn, 3); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated, err := getExperiment(ctx, nn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseRunning {
		t.Fatalf("expected Running, got %s", updated.Status.Phase)
	}

	if updated.Annotations == nil {
		updated.Annotations = make(map[string]string)
	}
	updated.Annotations["chaosplane.dev/abort"] = "true"
	if err := k8sClient.Update(ctx, updated); err != nil {
		t.Fatalf("update abort annotation: %v", err)
	}

	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile abort: %v", err)
	}

	updated, err = getExperiment(ctx, nn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseAborted {
		t.Fatalf("expected Aborted, got %s", updated.Status.Phase)
	}
	if !hasEvent(rec, "Aborted") {
		t.Fatal("expected Aborted event")
	}
}

func TestReconcileValidationFailure(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	exp := makeExperiment("test-notype", 30*time.Second)
	exp.Spec.Action.Type = "nonexistent-action"
	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r, rec := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := reconcileN(ctx, r, nn, 2); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated, err := getExperiment(ctx, nn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseFailed {
		t.Fatalf("expected Failed, got %s", updated.Status.Phase)
	}
	if !hasCondition(updated, "Degraded", metav1.ConditionTrue) {
		t.Fatal("expected Degraded=True")
	}
	if !hasEvent(rec, "Failed") {
		t.Fatal("expected Failed event")
	}
}

func TestReconcileRestartRecovery(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	exp := makeExperiment("test-restart", 1*time.Millisecond)
	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r, _ := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := reconcileN(ctx, r, nn, 2); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	r2, _ := newReconciler()

	if _, err := r2.Reconcile(ctx, reconcile.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile restart: %v", err)
	}

	updated, err := getExperiment(ctx, nn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseCompleting {
		t.Fatalf("expected Completing after restart, got %s", updated.Status.Phase)
	}

	if _, err := r2.Reconcile(ctx, reconcile.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile completing: %v", err)
	}

	updated, err = getExperiment(ctx, nn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseCompleted {
		t.Fatalf("expected Completed, got %s", updated.Status.Phase)
	}
}

func TestReconcileDeletionWhileRunning(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	exp := makeExperiment("test-delete", 5*time.Minute)
	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}

	r, rec := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := reconcileN(ctx, r, nn, 3); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated, err := getExperiment(ctx, nn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseRunning {
		t.Fatalf("expected Running, got %s", updated.Status.Phase)
	}

	if err := k8sClient.Delete(ctx, updated); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile deletion: %v", err)
	}

	if !hasEvent(rec, "RollingBack") {
		t.Fatal("expected RollingBack event during deletion")
	}
}

func TestReconcileRequeueAfterDuration(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	exp := makeExperiment("test-requeue", 10*time.Minute)
	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r, _ := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := reconcileN(ctx, r, nn, 2); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// execute — should return RequeueAfter with remaining duration
	res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
	if err != nil {
		t.Fatalf("reconcile running: %v", err)
	}

	if res.RequeueAfter <= 0 {
		t.Fatalf("expected RequeueAfter > 0, got %v", res.RequeueAfter)
	}
	if res.RequeueAfter > 10*time.Minute {
		t.Fatalf("RequeueAfter too large: %v", res.RequeueAfter)
	}
}

func TestReconcileConditionsAtEachPhase(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	exp := makeExperiment("test-conditions", 1*time.Millisecond)
	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r, _ := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := reconcileN(ctx, r, nn, 2); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated, _ := getExperiment(ctx, nn)
	if !hasCondition(updated, "Progressing", metav1.ConditionTrue) {
		t.Fatal("Running: expected Progressing=True")
	}
	if !hasCondition(updated, "Available", metav1.ConditionFalse) {
		t.Fatal("Running: expected Available=False")
	}
	if !hasCondition(updated, "Degraded", metav1.ConditionFalse) {
		t.Fatal("Running: expected Degraded=False")
	}

	time.Sleep(5 * time.Millisecond)

	if _, err := reconcileN(ctx, r, nn, 2); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated, _ = getExperiment(ctx, nn)
	if !hasCondition(updated, "Progressing", metav1.ConditionFalse) {
		t.Fatal("Completed: expected Progressing=False")
	}
	if !hasCondition(updated, "Available", metav1.ConditionTrue) {
		t.Fatal("Completed: expected Available=True")
	}
	if !hasCondition(updated, "Degraded", metav1.ConditionFalse) {
		t.Fatal("Completed: expected Degraded=False")
	}
}

func TestReconcileTerminalStatesAreNoOp(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	for _, phase := range []v1alpha1.ExperimentPhase{v1alpha1.PhaseCompleted, v1alpha1.PhaseFailed, v1alpha1.PhaseAborted} {
		t.Run(string(phase), func(t *testing.T) {
			exp := makeExperiment("test-terminal-"+strings.ToLower(string(phase)), 30*time.Second)
			ctx := context.Background()
			if err := k8sClient.Create(ctx, exp); err != nil {
				t.Fatalf("create: %v", err)
			}
			defer func() { _ = k8sClient.Delete(ctx, exp) }()

			r, _ := newReconciler()
			nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

			if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn}); err != nil {
				t.Fatalf("reconcile: %v", err)
			}

			updated, _ := getExperiment(ctx, nn)
			now := metav1.Now()
			updated.Status.Phase = phase
			updated.Status.EndTime = &now
			if err := k8sClient.Status().Update(ctx, updated); err != nil {
				t.Fatalf("status update: %v", err)
			}

			res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			if err != nil {
				t.Fatalf("reconcile terminal: %v", err)
			}
			if res.Requeue || res.RequeueAfter > 0 {
				t.Fatal("terminal state should not requeue")
			}
		})
	}
}

func promServer(value string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := fmt.Sprintf(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1234567890,%q]}]}}`, value)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, resp)
	}))
}

func promServerEmpty() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
}

func makeExperimentWithAbort(name string, duration time.Duration, abortConditions []v1alpha1.AbortConditionSpec) *v1alpha1.ChaosExperiment {
	exp := makeExperiment(name, duration)
	exp.Spec.AbortConditions = abortConditions
	return exp
}

func TestAbortConditions_PrometheusTriggered(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	srv := promServer("99.5")
	defer srv.Close()

	exp := makeExperimentWithAbort("test-abort-prom", 5*time.Minute, []v1alpha1.AbortConditionSpec{
		{
			Name: "high-error-rate",
			Type: v1alpha1.ProbeTypePrometheus,
			Prometheus: &v1alpha1.PrometheusProbe{
				URL:   srv.URL,
				Query: "error_rate",
				Condition: v1alpha1.ProbeCondition{
					Operator:  ">",
					Threshold: 50.0,
				},
			},
			Action: v1alpha1.AbortActionAbort,
		},
	})

	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r, rec := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := reconcileN(ctx, r, nn, 2); err != nil {
		t.Fatalf("reconcile to running: %v", err)
	}

	updated, _ := getExperiment(ctx, nn)
	if updated.Status.Phase != v1alpha1.PhaseRunning {
		t.Fatalf("expected Running, got %s", updated.Status.Phase)
	}

	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile running with abort: %v", err)
	}

	updated, _ = getExperiment(ctx, nn)
	if updated.Status.Phase != v1alpha1.PhaseAborted {
		t.Fatalf("expected Aborted, got %s", updated.Status.Phase)
	}
	if !hasEvent(rec, "AbortConditionTriggered") {
		t.Fatal("expected AbortConditionTriggered event")
	}
}

func TestAbortConditions_MultipleOR(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	srvLow := promServerEmpty()
	defer srvLow.Close()

	srvHigh := promServer("100")
	defer srvHigh.Close()

	exp := makeExperimentWithAbort("test-abort-multi", 5*time.Minute, []v1alpha1.AbortConditionSpec{
		{
			Name: "condition-a",
			Type: v1alpha1.ProbeTypePrometheus,
			Prometheus: &v1alpha1.PrometheusProbe{
				URL:   srvLow.URL,
				Query: "metric_a",
				Condition: v1alpha1.ProbeCondition{
					Operator:  ">",
					Threshold: 50.0,
				},
			},
			Action: v1alpha1.AbortActionAbort,
		},
		{
			Name: "condition-b",
			Type: v1alpha1.ProbeTypePrometheus,
			Prometheus: &v1alpha1.PrometheusProbe{
				URL:   srvHigh.URL,
				Query: "metric_b",
				Condition: v1alpha1.ProbeCondition{
					Operator:  ">",
					Threshold: 50.0,
				},
			},
			Action: v1alpha1.AbortActionAbort,
		},
	})

	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r, rec := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := reconcileN(ctx, r, nn, 2); err != nil {
		t.Fatalf("reconcile to running: %v", err)
	}

	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile running: %v", err)
	}

	updated, _ := getExperiment(ctx, nn)
	if updated.Status.Phase != v1alpha1.PhaseAborted {
		t.Fatalf("expected Aborted (OR eval), got %s", updated.Status.Phase)
	}
	if !hasEvent(rec, "AbortConditionTriggered") {
		t.Fatal("expected AbortConditionTriggered event")
	}
}

func TestAbortConditions_NoneTriggered(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	srv := promServer("10")
	defer srv.Close()

	exp := makeExperimentWithAbort("test-abort-none", 5*time.Minute, []v1alpha1.AbortConditionSpec{
		{
			Name: "low-error-rate",
			Type: v1alpha1.ProbeTypePrometheus,
			Prometheus: &v1alpha1.PrometheusProbe{
				URL:   srv.URL,
				Query: "error_rate",
				Condition: v1alpha1.ProbeCondition{
					Operator:  ">",
					Threshold: 50.0,
				},
			},
			Action: v1alpha1.AbortActionAbort,
		},
	})

	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r, _ := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := reconcileN(ctx, r, nn, 2); err != nil {
		t.Fatalf("reconcile to running: %v", err)
	}

	res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
	if err != nil {
		t.Fatalf("reconcile running: %v", err)
	}

	updated, _ := getExperiment(ctx, nn)
	if updated.Status.Phase != v1alpha1.PhaseRunning {
		t.Fatalf("expected Running (no trigger), got %s", updated.Status.Phase)
	}
	if res.RequeueAfter != 5*time.Second {
		t.Fatalf("expected 5s requeue with abort conditions, got %v", res.RequeueAfter)
	}
}

func TestAbortConditions_RollbackAction(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	srv := promServer("99.5")
	defer srv.Close()

	exp := makeExperimentWithAbort("test-abort-rb", 5*time.Minute, []v1alpha1.AbortConditionSpec{
		{
			Name: "high-error-rate",
			Type: v1alpha1.ProbeTypePrometheus,
			Prometheus: &v1alpha1.PrometheusProbe{
				URL:   srv.URL,
				Query: "error_rate",
				Condition: v1alpha1.ProbeCondition{
					Operator:  ">",
					Threshold: 50.0,
				},
			},
			Action: v1alpha1.AbortActionRollback,
		},
	})

	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r, rec := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := reconcileN(ctx, r, nn, 2); err != nil {
		t.Fatalf("reconcile to running: %v", err)
	}

	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile running: %v", err)
	}

	updated, _ := getExperiment(ctx, nn)
	if updated.Status.Phase != v1alpha1.PhaseCompleting {
		t.Fatalf("expected Completing (rollback action), got %s", updated.Status.Phase)
	}
	if !hasEvent(rec, "AbortConditionTriggered") {
		t.Fatal("expected AbortConditionTriggered event")
	}
}
