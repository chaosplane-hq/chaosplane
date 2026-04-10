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

func TestLossExecutor_Execute(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		networkResp: &daemonv1.NetworkChaosResponse{Success: true, ExecutionId: "exec-1"},
	}

	exec := network.NewLossExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("network-loss", map[string]string{"percent": "10%", "correlation": "25%"}, "victim")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastNetwork == nil || mc.lastNetwork.Action != "loss" {
		t.Fatal("expected network request with loss action")
	}
	if mc.lastNetwork.Parameters["percent"] != "10%" {
		t.Fatalf("expected percent=10%%, got %s", mc.lastNetwork.Parameters["percent"])
	}
}

func TestLossExecutor_Execute_DaemonError(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{networkErr: fmt.Errorf("daemon down")}

	exec := network.NewLossExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("network-loss", map[string]string{"percent": "10%"}, "victim")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error")
	}
}

func TestLossExecutor_Rollback(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		networkResp: &daemonv1.NetworkChaosResponse{Success: true, ExecutionId: "exec-1"},
		cancelResp:  &daemonv1.CancelResponse{Success: true},
	}

	exec := network.NewLossExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("network-loss", map[string]string{"percent": "10%"}, "victim")

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

func TestLossExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := network.NewLossExecutor(testLogger(), k8sClient, failingFactory())

	tests := []struct {
		name    string
		exp     func() *v1alpha1.ChaosExperiment
		wantErr bool
	}{
		{
			name: "valid",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("network-loss", map[string]string{"percent": "10%"}, "pod-1")
			},
		},
		{
			name: "missing percent",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("network-loss", map[string]string{}, "pod-1")
			},
			wantErr: true,
		},
		{
			name: "missing namespace",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newExp("network-loss", map[string]string{"percent": "10%"}, "pod-1")
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
