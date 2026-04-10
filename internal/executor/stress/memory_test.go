package stress_test

import (
	"context"
	"testing"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/stress"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMemoryExecutor_Execute(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: true, ExecutionId: "exec-mem-1"},
	}

	exec := stress.NewMemoryExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newStressExp("stress-memory", map[string]string{"workers": "2", "size": "256m", "duration": "60s"}, "node-1")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastStress == nil || mc.lastStress.StressorType != "memory" {
		t.Fatal("expected stress request with memory type")
	}
	if mc.lastStress.Parameters["size"] != "256m" {
		t.Fatalf("expected size=256m, got %s", mc.lastStress.Parameters["size"])
	}
}

func TestMemoryExecutor_Execute_NoTargets(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	mc := &mockDaemonClient{}

	exec := stress.NewMemoryExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newStressExp("stress-memory", nil, "nonexistent")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestMemoryExecutor_Execute_DaemonFailure(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: false, Message: "oom killed"},
	}

	exec := stress.NewMemoryExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newStressExp("stress-memory", map[string]string{"workers": "1", "size": "512m"}, "node-1")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for daemon failure")
	}
}

func TestMemoryExecutor_Execute_ConnectionRefused(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()

	exec := stress.NewMemoryExecutor(testLogger(), k8sClient, failingFactory())
	exp := newStressExp("stress-memory", nil, "node-1")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error")
	}
}

func TestMemoryExecutor_Rollback(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: true, ExecutionId: "exec-mem-1"},
		cancelResp: &daemonv1.CancelResponse{Success: true},
	}

	exec := stress.NewMemoryExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newStressExp("stress-memory", map[string]string{"workers": "1", "size": "256m"}, "node-1")

	_ = exec.Execute(context.Background(), exp)

	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastCancel == nil || mc.lastCancel.ExecutionId != "exec-mem-1" {
		t.Fatal("expected cancel with correct execution ID")
	}
}

func TestMemoryExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := stress.NewMemoryExecutor(testLogger(), k8sClient, failingFactory())

	tests := []struct {
		name    string
		exp     func() *v1alpha1.ChaosExperiment
		wantErr bool
	}{
		{
			name: "valid with names",
			exp: func() *v1alpha1.ChaosExperiment {
				return newStressExp("stress-memory", nil, "node-1")
			},
		},
		{
			name: "valid with selector",
			exp: func() *v1alpha1.ChaosExperiment {
				return newStressExpWithSelector("stress-memory", nil, map[string]string{"role": "worker"})
			},
		},
		{
			name: "missing target",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newStressExp("stress-memory", nil)
				e.Spec.Target.Names = nil
				return e
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := exec.Validate(tt.exp())
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
