package pod_test

import (
	"context"
	"testing"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDNSErrorExecutor_Execute(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		dnsResp: &daemonv1.DNSChaosResponse{Success: true, ExecutionId: "exec-dns-1"},
	}

	exec := pod.NewDNSErrorExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-dns-error", map[string]string{"domains": "example.com", "errorType": "NXDOMAIN"}, "victim")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastDNS == nil || mc.lastDNS.Action != "error" {
		t.Fatal("expected DNS request with error action")
	}
}

func TestDNSErrorExecutor_Rollback(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		dnsResp:    &daemonv1.DNSChaosResponse{Success: true, ExecutionId: "exec-dns-1"},
		cancelResp: &daemonv1.CancelResponse{Success: true},
	}

	exec := pod.NewDNSErrorExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-dns-error", map[string]string{"domains": "example.com"}, "victim")

	_ = exec.Execute(context.Background(), exp)
	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastCancel == nil || mc.lastCancel.ExecutionId != "exec-dns-1" {
		t.Fatal("expected cancel with correct execution ID")
	}
}

func TestDNSErrorExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := pod.NewDNSErrorExecutor(testLogger(), k8sClient, failingFactory())

	valid := newExp("pod-dns-error", nil, "pod-1")
	if err := exec.Validate(valid); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}

	noNs := newExp("pod-dns-error", nil, "pod-1")
	noNs.Spec.Target.Namespace = ""
	if err := exec.Validate(noNs); err == nil {
		t.Fatal("expected error for missing namespace")
	}
}
