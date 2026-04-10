package network_test

import (
	"context"
	"fmt"
	"testing"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/network"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPartitionExecutor_Execute(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		networkResp: &daemonv1.NetworkChaosResponse{Success: true, ExecutionId: "exec-1"},
	}

	exec := network.NewPartitionExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("network-partition", map[string]string{"target_cidr": "10.0.0.0/24", "direction": "both"}, "victim")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastNetwork == nil || mc.lastNetwork.Action != "partition" {
		t.Fatal("expected network request with partition action")
	}
	if mc.lastNetwork.Parameters["target_cidr"] != "10.0.0.0/24" {
		t.Fatalf("expected target_cidr=10.0.0.0/24, got %s", mc.lastNetwork.Parameters["target_cidr"])
	}
}

func TestPartitionExecutor_Execute_DaemonError(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{networkErr: fmt.Errorf("daemon down")}

	exec := network.NewPartitionExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("network-partition", map[string]string{"target_cidr": "10.0.0.0/24", "direction": "both"}, "victim")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error")
	}
}

func TestPartitionExecutor_Rollback(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		networkResp: &daemonv1.NetworkChaosResponse{Success: true, ExecutionId: "exec-1"},
		cancelResp:  &daemonv1.CancelResponse{Success: true},
	}

	exec := network.NewPartitionExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("network-partition", map[string]string{"target_cidr": "10.0.0.0/24", "direction": "both"}, "victim")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if mc.lastCancel == nil || mc.lastCancel.ExecutionId != "exec-1" {
		t.Fatal("expected cancel with exec-1")
	}
}

func TestPartitionExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := network.NewPartitionExecutor(testLogger(), k8sClient, failingFactory())

	tests := []struct {
		name    string
		exp     func() *v1alpha1.ChaosExperiment
		wantErr bool
	}{
		{
			name: "valid",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("network-partition", map[string]string{"target_cidr": "10.0.0.0/24", "direction": "both"}, "pod-1")
			},
		},
		{
			name: "missing target_cidr",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("network-partition", map[string]string{"direction": "both"}, "pod-1")
			},
			wantErr: true,
		},
		{
			name: "missing direction",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("network-partition", map[string]string{"target_cidr": "10.0.0.0/24"}, "pod-1")
			},
			wantErr: true,
		},
		{
			name: "invalid direction",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("network-partition", map[string]string{"target_cidr": "10.0.0.0/24", "direction": "invalid"}, "pod-1")
			},
			wantErr: true,
		},
		{
			name: "missing namespace",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newExp("network-partition", map[string]string{"target_cidr": "10.0.0.0/24", "direction": "egress"}, "pod-1")
				e.Spec.Target.Namespace = ""
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
