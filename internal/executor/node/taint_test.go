package node_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/node"
)

func TestTaintExecutor_Execute(t *testing.T) {
	n := testNode("node-1")
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()

	exec := node.NewTaintExecutor(testLogger(), k8sClient)
	exp := newNodeExp("node-taint", map[string]string{
		"key": "chaos", "value": "true", "effect": "NoSchedule",
	}, "node-1")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var updated corev1.Node
	if err := k8sClient.Get(context.Background(), client_key("node-1"), &updated); err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	found := false
	for _, taint := range updated.Spec.Taints {
		if taint.Key == "chaos" && taint.Effect == corev1.TaintEffectNoSchedule {
			found = true
		}
	}
	if !found {
		t.Fatal("expected taint to be added")
	}
}

func TestTaintExecutor_Execute_Idempotent(t *testing.T) {
	n := testNode("node-1")
	n.Spec.Taints = []corev1.Taint{
		{Key: "chaos", Value: "true", Effect: corev1.TaintEffectNoSchedule},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()

	exec := node.NewTaintExecutor(testLogger(), k8sClient)
	exp := newNodeExp("node-taint", map[string]string{
		"key": "chaos", "value": "true", "effect": "NoSchedule",
	}, "node-1")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var updated corev1.Node
	if err := k8sClient.Get(context.Background(), client_key("node-1"), &updated); err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	count := 0
	for _, taint := range updated.Spec.Taints {
		if taint.Key == "chaos" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 taint, got %d", count)
	}
}

func TestTaintExecutor_Execute_NodeNotFound(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()

	exec := node.NewTaintExecutor(testLogger(), k8sClient)
	exp := newNodeExp("node-taint", map[string]string{
		"key": "chaos", "value": "true", "effect": "NoSchedule",
	}, "nonexistent")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestTaintExecutor_Rollback(t *testing.T) {
	n := testNode("node-1")
	n.Spec.Taints = []corev1.Taint{
		{Key: "chaos", Value: "true", Effect: corev1.TaintEffectNoSchedule},
		{Key: "other", Value: "val", Effect: corev1.TaintEffectNoExecute},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()

	exec := node.NewTaintExecutor(testLogger(), k8sClient)
	exp := newNodeExp("node-taint", map[string]string{
		"key": "chaos", "value": "true", "effect": "NoSchedule",
	}, "node-1")

	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var updated corev1.Node
	if err := k8sClient.Get(context.Background(), client_key("node-1"), &updated); err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	for _, taint := range updated.Spec.Taints {
		if taint.Key == "chaos" {
			t.Fatal("expected chaos taint to be removed")
		}
	}
	if len(updated.Spec.Taints) != 1 {
		t.Fatalf("expected 1 remaining taint, got %d", len(updated.Spec.Taints))
	}
}

func TestTaintExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	exec := node.NewTaintExecutor(testLogger(), k8sClient)

	tests := []struct {
		name    string
		exp     func() *v1alpha1.ChaosExperiment
		wantErr bool
	}{
		{
			name: "valid",
			exp: func() *v1alpha1.ChaosExperiment {
				return newNodeExp("node-taint", map[string]string{
					"key": "chaos", "effect": "NoSchedule",
				}, "node-1")
			},
		},
		{
			name: "missing key",
			exp: func() *v1alpha1.ChaosExperiment {
				return newNodeExp("node-taint", map[string]string{
					"effect": "NoSchedule",
				}, "node-1")
			},
			wantErr: true,
		},
		{
			name: "invalid effect",
			exp: func() *v1alpha1.ChaosExperiment {
				return newNodeExp("node-taint", map[string]string{
					"key": "chaos", "effect": "Invalid",
				}, "node-1")
			},
			wantErr: true,
		},
		{
			name: "missing target",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newNodeExp("node-taint", map[string]string{
					"key": "chaos", "effect": "NoSchedule",
				})
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
