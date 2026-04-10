package pod_test

import (
	"context"
	"log/slog"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

func newTestExperiment() *v1alpha1.ChaosExperiment {
	return &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-exp",
			Namespace: "default",
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Action: v1alpha1.ActionSpec{Type: "pod-kill"},
		},
	}
}

func TestKillExecutorExecute(t *testing.T) {
	exec := pod.NewKillExecutor(slog.Default())
	if err := exec.Execute(context.Background(), newTestExperiment()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestKillExecutorRollback(t *testing.T) {
	exec := pod.NewKillExecutor(slog.Default())
	if err := exec.Rollback(context.Background(), newTestExperiment()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestKillExecutorValidate(t *testing.T) {
	exec := pod.NewKillExecutor(slog.Default())
	if err := exec.Validate(newTestExperiment()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
