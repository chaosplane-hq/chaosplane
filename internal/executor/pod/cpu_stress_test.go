package pod_test

import (
	"context"
	"fmt"
	"testing"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCPUStressExecutor_Execute(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: true, ExecutionId: "exec-cpu-1"},
	}

	exec := pod.NewCPUStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-cpu-stress", map[string]string{"workers": "2", "load": "80", "duration": "30s"}, "victim")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastStress == nil || mc.lastStress.StressorType != "cpu" {
		t.Fatal("expected stress request with cpu type")
	}
}

func TestCPUStressExecutor_Execute_DaemonFailure(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: false, Message: "stress-ng not found"},
	}

	exec := pod.NewCPUStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-cpu-stress", map[string]string{"workers": "2"}, "victim")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for daemon failure")
	}
}

func TestCPUStressExecutor_Rollback(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		stressResp: &daemonv1.StressChaosResponse{Success: true, ExecutionId: "exec-cpu-1"},
		cancelResp: &daemonv1.CancelResponse{Success: true},
	}

	exec := pod.NewCPUStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-cpu-stress", map[string]string{"workers": "2"}, "victim")

	_ = exec.Execute(context.Background(), exp)

	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastCancel == nil || mc.lastCancel.ExecutionId != "exec-cpu-1" {
		t.Fatal("expected cancel with correct execution ID")
	}
}

func TestCPUStressExecutor_Execute_ConnectionRefused(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()

	exec := pod.NewCPUStressExecutor(testLogger(), k8sClient, failingFactory())
	exp := newExp("pod-cpu-stress", nil, "victim")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error")
	}
}

func TestCPUStressExecutor_Execute_TargetNotFound(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	mc := &mockDaemonClient{stressErr: fmt.Errorf("unused")}

	exec := pod.NewCPUStressExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-cpu-stress", nil, "nonexistent")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for missing pod")
	}
}

func TestCPUStressExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := pod.NewCPUStressExecutor(testLogger(), k8sClient, failingFactory())

	valid := newExp("pod-cpu-stress", nil, "pod-1")
	if err := exec.Validate(valid); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}

	noNs := newExp("pod-cpu-stress", nil, "pod-1")
	noNs.Spec.Target.Namespace = ""
	if err := exec.Validate(noNs); err == nil {
		t.Fatal("expected error for missing namespace")
	}
}
