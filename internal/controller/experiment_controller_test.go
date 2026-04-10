package controller_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

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

func newReconciler() *controller.ExperimentReconciler {
	registry := executor.NewRegistry()
	registry.Register("pod-kill", pod.NewKillExecutor(slog.Default(), k8sClient, fake.NewSimpleClientset()))
	return &controller.ExperimentReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		Recorder: &fakeRecorder{},
		Registry: registry,
		Logger:   slog.Default(),
	}
}

type fakeRecorder struct{}

func (f *fakeRecorder) Event(_ runtime.Object, _, _, _ string)                    {}
func (f *fakeRecorder) Eventf(_ runtime.Object, _, _, _ string, _ ...interface{}) {}
func (f *fakeRecorder) AnnotatedEventf(_ runtime.Object, _ map[string]string, _, _, _ string, _ ...interface{}) {
}

func TestReconcilePendingToRunning(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pending-" + time.Now().Format("150405"),
			Namespace: "default",
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:   v1alpha1.TargetSpec{Kind: "Pod", Namespace: "default"},
			Action:   v1alpha1.ActionSpec{Type: "pod-kill"},
			Duration: metav1.Duration{Duration: 30 * time.Second},
		},
	}

	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("failed to create experiment: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var updated v1alpha1.ChaosExperiment
	if err := k8sClient.Get(ctx, nn, &updated); err != nil {
		t.Fatalf("get error: %v", err)
	}

	hasFinalizer := false
	for _, f := range updated.Finalizers {
		if f == "chaosplane.io/experiment-protection" {
			hasFinalizer = true
			break
		}
	}
	if !hasFinalizer {
		t.Fatal("expected finalizer to be added")
	}

	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	if err := k8sClient.Get(ctx, nn, &updated); err != nil {
		t.Fatalf("get error: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseRunning {
		t.Fatalf("expected Running, got %s", updated.Status.Phase)
	}
	if updated.Status.StartTime == nil {
		t.Fatal("expected startTime to be set")
	}
}

func TestReconcileRunningToCompleted(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available")
	}

	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-complete-" + time.Now().Format("150405"),
			Namespace: "default",
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:   v1alpha1.TargetSpec{Kind: "Pod", Namespace: "default"},
			Action:   v1alpha1.ActionSpec{Type: "pod-kill"},
			Duration: metav1.Duration{Duration: 30 * time.Second},
		},
	}

	ctx := context.Background()
	if err := k8sClient.Create(ctx, exp); err != nil {
		t.Fatalf("failed to create experiment: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, exp) }()

	r := newReconciler()
	nn := types.NamespacedName{Name: exp.Name, Namespace: exp.Namespace}

	_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
	_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn}); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var updated v1alpha1.ChaosExperiment
	if err := k8sClient.Get(ctx, nn, &updated); err != nil {
		t.Fatalf("get error: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseCompleted {
		t.Fatalf("expected Completed, got %s", updated.Status.Phase)
	}
	if updated.Status.EndTime == nil {
		t.Fatal("expected endTime to be set")
	}
}
