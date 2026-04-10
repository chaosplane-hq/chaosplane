package pod_test

import (
	"context"
	"fmt"
	"testing"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestContainerKillExecutor_Execute(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: true, ExecutionId: "exec-1"},
	}

	exec := pod.NewContainerKillExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("container-kill", map[string]string{"containerName": "app"}, "victim")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastStress == nil || mc.lastStress.StressorType != "container-kill" {
		t.Fatal("expected stress request with container-kill type")
	}
	if mc.lastStress.Parameters["containerName"] != "app" {
		t.Fatalf("expected containerName=app, got %s", mc.lastStress.Parameters["containerName"])
	}
}

func TestContainerKillExecutor_Execute_DaemonError(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{stressErr: fmt.Errorf("daemon down")}

	exec := pod.NewContainerKillExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("container-kill", map[string]string{"containerName": "app"}, "victim")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error")
	}
}

func TestContainerKillExecutor_Rollback(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := pod.NewContainerKillExecutor(testLogger(), k8sClient, failingFactory())
	exp := newExp("container-kill", nil, "victim")

	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("rollback should be no-op, got %v", err)
	}
}

func TestContainerKillExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := pod.NewContainerKillExecutor(testLogger(), k8sClient, failingFactory())

	tests := []struct {
		name    string
		exp     func() *v1alpha1.ChaosExperiment
		wantErr bool
	}{
		{
			name: "valid",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("container-kill", map[string]string{"containerName": "app"}, "pod-1")
			},
		},
		{
			name: "missing containerName",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("container-kill", map[string]string{}, "pod-1")
			},
			wantErr: true,
		},
		{
			name: "missing namespace",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newExp("container-kill", map[string]string{"containerName": "app"}, "pod-1")
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
