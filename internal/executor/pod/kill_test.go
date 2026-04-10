package pod_test

import (
	"context"
	"testing"

	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

func TestKillExecutor_Execute(t *testing.T) {
	p := testPod("victim-pod", "default", "node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	cs := fakeclientset.NewSimpleClientset(p)

	exec := pod.NewKillExecutor(testLogger(), k8sClient, cs)
	exp := newExp("pod-kill", map[string]string{"gracePeriodSeconds": "0"}, "victim-pod")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestKillExecutor_Execute_PodNotFound(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	cs := fakeclientset.NewSimpleClientset()

	exec := pod.NewKillExecutor(testLogger(), k8sClient, cs)
	exp := newExp("pod-kill", nil, "nonexistent")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for missing pod")
	}
}

func TestKillExecutor_Rollback(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	cs := fakeclientset.NewSimpleClientset()

	exec := pod.NewKillExecutor(testLogger(), k8sClient, cs)
	exp := newExp("pod-kill", nil, "victim-pod")

	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("rollback should be no-op, got %v", err)
	}
}

func TestKillExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	cs := fakeclientset.NewSimpleClientset()
	exec := pod.NewKillExecutor(testLogger(), k8sClient, cs)

	tests := []struct {
		name    string
		exp     func() *v1alpha1.ChaosExperiment
		wantErr bool
	}{
		{
			name: "valid with names",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExp("pod-kill", nil, "pod-1")
			},
		},
		{
			name: "valid with selector",
			exp: func() *v1alpha1.ChaosExperiment {
				return newExpWithSelector("pod-kill", nil, map[string]string{"app": "test"})
			},
		},
		{
			name: "missing namespace",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newExp("pod-kill", nil, "pod-1")
				e.Spec.Target.Namespace = ""
				return e
			},
			wantErr: true,
		},
		{
			name: "missing target",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newExp("pod-kill", nil)
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
