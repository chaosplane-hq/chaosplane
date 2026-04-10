package node_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/node"
)

func TestDrainExecutor_Execute(t *testing.T) {
	n := testNode("node-1")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "victim",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{NodeName: "node-1"},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()
	cs := fakeclientset.NewSimpleClientset(n, pod)

	exec := node.NewDrainExecutor(testLogger(), k8sClient, cs)
	exp := newNodeExp("node-drain", map[string]string{"timeout": "30s"}, "node-1")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var updated corev1.Node
	if err := k8sClient.Get(context.Background(), client_key("node-1"), &updated); err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	if !updated.Spec.Unschedulable {
		t.Fatal("expected node to be cordoned (Unschedulable=true)")
	}
}

func TestDrainExecutor_Execute_NodeNotFound(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	cs := fakeclientset.NewSimpleClientset()

	exec := node.NewDrainExecutor(testLogger(), k8sClient, cs)
	exp := newNodeExp("node-drain", nil, "nonexistent")

	if err := exec.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestDrainExecutor_Rollback(t *testing.T) {
	n := testNode("node-1")
	n.Spec.Unschedulable = true
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()
	cs := fakeclientset.NewSimpleClientset(n)

	exec := node.NewDrainExecutor(testLogger(), k8sClient, cs)
	exp := newNodeExp("node-drain", nil, "node-1")

	if err := exec.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var updated corev1.Node
	if err := k8sClient.Get(context.Background(), client_key("node-1"), &updated); err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	if updated.Spec.Unschedulable {
		t.Fatal("expected node to be uncordoned (Unschedulable=false)")
	}
}

func TestDrainExecutor_Validate(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	cs := fakeclientset.NewSimpleClientset()
	exec := node.NewDrainExecutor(testLogger(), k8sClient, cs)

	tests := []struct {
		name    string
		exp     func() *v1alpha1.ChaosExperiment
		wantErr bool
	}{
		{
			name: "valid with names",
			exp: func() *v1alpha1.ChaosExperiment {
				return newNodeExp("node-drain", nil, "node-1")
			},
		},
		{
			name: "valid with selector",
			exp: func() *v1alpha1.ChaosExperiment {
				return newNodeExpWithSelector("node-drain", nil, map[string]string{"role": "worker"})
			},
		},
		{
			name: "missing target",
			exp: func() *v1alpha1.ChaosExperiment {
				e := newNodeExp("node-drain", nil)
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

func TestDrainExecutor_Execute_SkipsDaemonSetPods(t *testing.T) {
	n := testNode("node-1")
	dsPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds-pod",
			Namespace: "kube-system",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "DaemonSet", Name: "kube-proxy"},
			},
		},
		Spec: corev1.PodSpec{NodeName: "node-1"},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(n).Build()
	cs := fakeclientset.NewSimpleClientset(n, dsPod)

	exec := node.NewDrainExecutor(testLogger(), k8sClient, cs)
	exp := newNodeExp("node-drain", map[string]string{"ignoreDaemonSets": "true"}, "node-1")

	if err := exec.Execute(context.Background(), exp); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
