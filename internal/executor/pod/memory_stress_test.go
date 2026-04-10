package pod_test

import (
	"context"
	"testing"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMemoryStressExecutor_Execute(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: true, ExecutionId: "exec-mem-1"},
	}

	exec := pod.NewMemoryStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-memory-stress", map[string]string{"workers": "1", "size": "256M", "duration": "30s"}, "victim")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastStress == nil || mc.lastStress.StressorType != "memory" {
		t.Fatal("expected stress request with memory type")
	}
}

func TestMemoryStressExecutor_Rollback(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: true, ExecutionId: "exec-mem-1"},
		cancelResp: &daemonv1.CancelResponse{Success: true},
	}

	exec := pod.NewMemoryStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-memory-stress", map[string]string{"workers": "1"}, "victim")

	_ = exec.Execute(context.Background(), exp)
	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastCancel == nil || mc.lastCancel.ExecutionId != "exec-mem-1" {
		t.Fatal("expected cancel with correct execution ID")
	}
}

func TestMemoryStressExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := pod.NewMemoryStressExecutor(testLogger(), k8sClient, failingFactory())

	valid := newExp("pod-memory-stress", nil, "pod-1")
	if err := exec.Validate(valid); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}

	noNs := newExp("pod-memory-stress", nil, "pod-1")
	noNs.Spec.Target.Namespace = ""
	if err := exec.Validate(noNs); err == nil {
		t.Fatal("expected error for missing namespace")
	}
}
