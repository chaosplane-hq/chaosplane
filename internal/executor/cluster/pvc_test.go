package cluster_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/chaosplane-hq/chaosplane/internal/executor/cluster"
)

func makePVC(name, ns string) *corev1.PersistentVolumeClaim {
	sc := "standard"
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{"app": "db"}},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &sc,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
}

func TestPVCChaos_DeleteAndRecreate(t *testing.T) {
	pvc := makePVC("data", "default")
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(pvc).Build()
	e := cluster.NewPVCChaosExecutor(testLogger(), c)
	exp := newExp("pvc-chaos", "default", "data", nil)

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var gone corev1.PersistentVolumeClaim
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "data"}, &gone); !apierrors.IsNotFound(err) {
		t.Fatalf("expected pvc deleted, got err=%v", err)
	}

	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	var restored corev1.PersistentVolumeClaim
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "data"}, &restored); err != nil {
		t.Fatalf("expected pvc recreated, got %v", err)
	}
	if restored.Labels["app"] != "db" {
		t.Fatalf("expected labels preserved, got %v", restored.Labels)
	}
	if *restored.Spec.StorageClassName != "standard" {
		t.Fatalf("expected storageclass preserved, got %v", restored.Spec.StorageClassName)
	}
	if restored.Spec.Resources.Requests.Storage().String() != "1Gi" {
		t.Fatalf("expected 1Gi request preserved, got %v", restored.Spec.Resources.Requests.Storage())
	}
}

func TestPVCChaos_RollbackSkipsWhenStillPresent(t *testing.T) {
	pvc := makePVC("data", "default")
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(pvc).Build()
	e := cluster.NewPVCChaosExecutor(testLogger(), c)
	exp := newExp("pvc-chaos", "default", "data", nil)

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Simulate the PVC reappearing (finalizer still holding the original); rollback
	// must not error or clobber it.
	recreate := makePVC("data", "default")
	if err := c.Create(context.Background(), recreate); err != nil {
		t.Fatalf("recreate setup: %v", err)
	}
	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
}

func TestPVCChaos_MissingTargetFails(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme).Build()
	e := cluster.NewPVCChaosExecutor(testLogger(), c)
	exp := newExp("pvc-chaos", "default", "nope", nil)
	if err := e.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for missing pvc")
	}
}

func TestPVCChaos_Validate(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme).Build()
	e := cluster.NewPVCChaosExecutor(testLogger(), c)
	if err := e.Validate(newExp("pvc-chaos", "default", "data", nil)); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	bad := newExp("pvc-chaos", "", "data", nil)
	if err := e.Validate(bad); err == nil {
		t.Fatal("expected error for missing namespace")
	}
}
