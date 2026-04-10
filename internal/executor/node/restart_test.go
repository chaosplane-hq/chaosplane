package node_test

import (
	"context"
	"testing"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/node"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRestartExecutor_Execute(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()
	mc := &mockDaemonClient{
		nodeResp: &daemonv1.NodeChaosResponse{Success: true, ExecutionId: "exec-restart-1"},
	}

	exec := node.NewRestartExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newNodeExp("node-restart", map[string]string{"grace_period": "30"}, "node-1")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastNode == nil || mc.lastNode.Action != "restart" {
		t.Fatal("expected node chaos request with restart action")
	}
}

func TestRestartExecutor_Execute_DaemonFailure(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()
	mc := &mockDaemonClient{
		nodeResp: &daemonv1.NodeChaosResponse{Success: false, Message: "reboot failed"},
	}

	exec := node.NewRestartExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newNodeExp("node-restart", nil, "node-1")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for daemon failure")
	}
}

func TestRestartExecutor_Execute_ConnectionRefused(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()

	exec := node.NewRestartExecutor(testLogger(), k8sClient, failingFactory())
	exp := newNodeExp("node-restart", nil, "node-1")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error")
	}
}

func TestRestartExecutor_Execute_NodeNotFound(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	mc := &mockDaemonClient{}

	exec := node.NewRestartExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newNodeExp("node-restart", nil, "nonexistent")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestRestartExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := node.NewRestartExecutor(testLogger(), k8sClient, failingFactory())

	tests := []struct {
		name    string
		exp     func() *v1alpha1.ChaosExperiment
		wantErr bool
	}{
		{
			name: "valid with names",
			exp: func() *v1alpha1.ChaosExperiment {
				return newNodeExp("node-restart", nil, "node-1")
			},
		},
		{
			name: "valid with selector",
			exp: func() *v1alpha1.ChaosExperiment {
				return newNodeExpWithSelector("node-restart", nil, map[string]string{"role": "worker"})
			},
		},
		{
			name: "missing target",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newNodeExp("node-restart", nil)
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
