package node_test

import (
	"context"
	"testing"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/node"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCPUStressExecutor_Execute(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: true, ExecutionId: "exec-cpu-1"},
	}

	exec := node.NewCPUStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newNodeExp("node-cpu-stress", map[string]string{"workers": "4", "load": "90", "duration": "60s"}, "node-1")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastStress == nil || mc.lastStress.StressorType != "cpu" {
		t.Fatal("expected stress request with cpu type")
	}
}

func TestCPUStressExecutor_Execute_DaemonFailure(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: false, Message: "stress-ng not found"},
	}

	exec := node.NewCPUStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newNodeExp("node-cpu-stress", map[string]string{"workers": "2"}, "node-1")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for daemon failure")
	}
}

func TestCPUStressExecutor_Rollback(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: true, ExecutionId: "exec-cpu-1"},
		cancelResp: &daemonv1.CancelResponse{Success: true},
	}

	exec := node.NewCPUStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newNodeExp("node-cpu-stress", map[string]string{"workers": "2"}, "node-1")

	_ = exec.Execute(context.Background(), exp)

	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastCancel == nil || mc.lastCancel.ExecutionId != "exec-cpu-1" {
		t.Fatal("expected cancel with correct execution ID")
	}
}

func TestCPUStressExecutor_Execute_ConnectionRefused(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()

	exec := node.NewCPUStressExecutor(testLogger(), k8sClient, failingFactory())
	exp := newNodeExp("node-cpu-stress", nil, "node-1")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error")
	}
}

func TestCPUStressExecutor_Execute_NodeNotFound(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	mc := &mockDaemonClient{}

	exec := node.NewCPUStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newNodeExp("node-cpu-stress", nil, "nonexistent")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestCPUStressExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := node.NewCPUStressExecutor(testLogger(), k8sClient, failingFactory())

	tests := []struct {
		name    string
		exp     func() *v1alpha1.ChaosExperiment
		wantErr bool
	}{
		{
			name: "valid with names",
			exp: func() *v1alpha1.ChaosExperiment {
				return newNodeExp("node-cpu-stress", nil, "node-1")
			},
		},
		{
			name: "valid with selector",
			exp: func() *v1alpha1.ChaosExperiment {
				return newNodeExpWithSelector("node-cpu-stress", nil, map[string]string{"role": "worker"})
			},
		},
		{
			name: "missing target",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newNodeExp("node-cpu-stress", nil)
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
