package cluster_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/chaosplane-hq/chaosplane/internal/executor/cluster"
)

func makeCM(name, ns string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       data,
	}
}

func TestConfigMapChaos_Corrupt_RestoresOriginal(t *testing.T) {
	cm := makeCM("app-config", "default", map[string]string{"url": "real", "key": "secret"})
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(cm).Build()
	e := cluster.NewConfigMapChaosExecutor(testLogger(), c)
	exp := newExp("configmap-chaos", "default", "app-config", map[string]string{"mode": "corrupt", "value": "GARBAGE"})

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got corev1.ConfigMap
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "app-config"}, &got); err != nil {
		t.Fatalf("get after execute: %v", err)
	}
	if got.Data["url"] != "GARBAGE" || got.Data["key"] != "GARBAGE" {
		t.Fatalf("expected all values corrupted, got %v", got.Data)
	}

	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	var restored corev1.ConfigMap
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "app-config"}, &restored); err != nil {
		t.Fatalf("get after rollback: %v", err)
	}
	if restored.Data["url"] != "real" || restored.Data["key"] != "secret" {
		t.Fatalf("expected original data restored, got %v", restored.Data)
	}
}

func TestConfigMapChaos_CorruptSingleKey(t *testing.T) {
	cm := makeCM("app-config", "default", map[string]string{"url": "real", "key": "secret"})
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(cm).Build()
	e := cluster.NewConfigMapChaosExecutor(testLogger(), c)
	exp := newExp("configmap-chaos", "default", "app-config", map[string]string{"key": "url", "value": "X"})

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got corev1.ConfigMap
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "app-config"}, &got)
	if got.Data["url"] != "X" || got.Data["key"] != "secret" {
		t.Fatalf("expected only url corrupted, got %v", got.Data)
	}
}

func TestConfigMapChaos_Delete_RecreatesOnRollback(t *testing.T) {
	cm := makeCM("app-config", "default", map[string]string{"url": "real"})
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(cm).Build()
	e := cluster.NewConfigMapChaosExecutor(testLogger(), c)
	exp := newExp("configmap-chaos", "default", "app-config", map[string]string{"mode": "delete"})

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var gone corev1.ConfigMap
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "app-config"}, &gone); !apierrors.IsNotFound(err) {
		t.Fatalf("expected configmap deleted, got err=%v", err)
	}

	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	var restored corev1.ConfigMap
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "app-config"}, &restored); err != nil {
		t.Fatalf("expected configmap recreated, got %v", err)
	}
	if restored.Data["url"] != "real" {
		t.Fatalf("expected original data on recreated cm, got %v", restored.Data)
	}
}

func TestConfigMapChaos_MissingTargetFails(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme).Build()
	e := cluster.NewConfigMapChaosExecutor(testLogger(), c)
	exp := newExp("configmap-chaos", "default", "nope", nil)
	if err := e.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for missing configmap")
	}
}

func TestConfigMapChaos_Validate(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme).Build()
	e := cluster.NewConfigMapChaosExecutor(testLogger(), c)
	if err := e.Validate(newExp("configmap-chaos", "default", "cm", nil)); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	bad := newExp("configmap-chaos", "default", "cm", nil)
	bad.Spec.Target.Names = nil
	if err := e.Validate(bad); err == nil {
		t.Fatal("expected error for missing name")
	}
}
