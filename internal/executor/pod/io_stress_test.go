package pod_test

import (
	"context"
	"testing"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIOStressExecutor_Execute(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: true, ExecutionId: "exec-io-1"},
	}

	exec := pod.NewIOStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-io-stress", map[string]string{"workers": "4", "size": "1G", "duration": "60s"}, "victim")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastStress == nil || mc.lastStress.StressorType != "io" {
		t.Fatal("expected stress request with io type")
	}
}

func TestIOStressExecutor_Rollback(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: true, ExecutionId: "exec-io-1"},
		cancelResp: &daemonv1.CancelResponse{Success: true},
	}

	exec := pod.NewIOStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-io-stress", map[string]string{"workers": "4"}, "victim")

	_ = exec.Execute(context.Background(), exp)
	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastCancel == nil || mc.lastCancel.ExecutionId != "exec-io-1" {
		t.Fatal("expected cancel with correct execution ID")
	}
}

func TestIOStressExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := pod.NewIOStressExecutor(testLogger(), k8sClient, failingFactory())

	valid := newExp("pod-io-stress", nil, "pod-1")
	if err := exec.Validate(valid); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}

	noNs := newExp("pod-io-stress", nil, "pod-1")
	noNs.Spec.Target.Namespace = ""
	if err := exec.Validate(noNs); err == nil {
		t.Fatal("expected error for missing namespace")
	}
}
