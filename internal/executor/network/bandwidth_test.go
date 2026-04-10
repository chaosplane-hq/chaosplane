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

func TestBandwidthExecutor_Execute(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		networkResp: &daemonv1.NetworkChaosResponse{Success: true, ExecutionId: "exec-1"},
	}

	exec := network.NewBandwidthExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("network-bandwidth", map[string]string{"rate": "1mbit", "burst": "32kbit", "latency": "400ms"}, "victim")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastNetwork == nil || mc.lastNetwork.Action != "bandwidth" {
		t.Fatal("expected network request with bandwidth action")
	}
	if mc.lastNetwork.Parameters["rate"] != "1mbit" {
		t.Fatalf("expected rate=1mbit, got %s", mc.lastNetwork.Parameters["rate"])
	}
}

func TestBandwidthExecutor_Execute_DaemonError(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{networkErr: fmt.Errorf("daemon down")}

	exec := network.NewBandwidthExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("network-bandwidth", map[string]string{"rate": "1mbit", "burst": "32kbit", "latency": "400ms"}, "victim")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error")
	}
}

func TestBandwidthExecutor_Rollback(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		networkResp: &daemonv1.NetworkChaosResponse{Success: true, ExecutionId: "exec-1"},
		cancelResp:  &daemonv1.CancelResponse{Success: true},
	}

	exec := network.NewBandwidthExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("network-bandwidth", map[string]string{"rate": "1mbit", "burst": "32kbit", "latency": "400ms"}, "victim")

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

func TestBandwidthExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := network.NewBandwidthExecutor(testLogger(), k8sClient, failingFactory())

	tests := []struct {
		name    string
		exp     func() *v1alpha1.ChaosExperiment
		wantErr bool
	}{
		{
			name: "valid",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("network-bandwidth", map[string]string{"rate": "1mbit", "burst": "32kbit", "latency": "400ms"}, "pod-1")
			},
		},
		{
			name: "missing rate",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("network-bandwidth", map[string]string{"burst": "32kbit", "latency": "400ms"}, "pod-1")
			},
			wantErr: true,
		},
		{
			name: "missing burst",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("network-bandwidth", map[string]string{"rate": "1mbit", "latency": "400ms"}, "pod-1")
			},
			wantErr: true,
		},
		{
			name: "missing latency",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("network-bandwidth", map[string]string{"rate": "1mbit", "burst": "32kbit"}, "pod-1")
			},
			wantErr: true,
		},
		{
			name: "missing namespace",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newExp("network-bandwidth", map[string]string{"rate": "1mbit", "burst": "32kbit", "latency": "400ms"}, "pod-1")
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
