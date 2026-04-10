package pod_test

import (
	"context"
	"testing"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHTTPDelayExecutor_Execute(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		httpResp: &daemonv1.HTTPChaosResponse{Success: true, ExecutionId: "exec-http-d-1"},
	}

	exec := pod.NewHTTPDelayExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-http-delay", map[string]string{"port": "8080", "delay": "500ms"}, "victim")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastHTTP == nil || mc.lastHTTP.Action != "delay" {
		t.Fatal("expected HTTP request with delay action")
	}
	if mc.lastHTTP.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", mc.lastHTTP.Port)
	}
}

func TestHTTPDelayExecutor_Rollback(t *testing.T) {
	p := testPod("victim", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	mc := &mockDaemonClient{
		httpResp:   &daemonv1.HTTPChaosResponse{Success: true, ExecutionId: "exec-http-d-1"},
		cancelResp: &daemonv1.CancelResponse{Success: true},
	}

	exec := pod.NewHTTPDelayExecutor(testLogger(), k8sClient, mockFactory(mc))
	exp := newExp("pod-http-delay", map[string]string{"port": "8080", "delay": "500ms"}, "victim")

	_ = exec.Execute(context.Background(), exp)
	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if mc.lastCancel == nil || mc.lastCancel.ExecutionId != "exec-http-d-1" {
		t.Fatal("expected cancel with correct execution ID")
	}
}

func TestHTTPDelayExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := pod.NewHTTPDelayExecutor(testLogger(), k8sClient, failingFactory())

	tests := []struct {
		name    string
		exp     func() *v1alpha1.ChaosExperiment
		wantErr bool
	}{
		{
			name: "valid",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("pod-http-delay", map[string]string{"port": "8080", "delay": "500ms"}, "pod-1")
			},
		},
		{
			name: "missing port",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("pod-http-delay", map[string]string{"delay": "500ms"}, "pod-1")
			},
			wantErr: true,
		},
		{
			name: "missing delay",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("pod-http-delay", map[string]string{"port": "8080"}, "pod-1")
			},
			wantErr: true,
		},
		{
			name: "missing namespace",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newExp("pod-http-delay", map[string]string{"port": "8080", "delay": "500ms"}, "pod-1")
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
