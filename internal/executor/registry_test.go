package executor_test

import (
	"testing"

	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := executor.NewRegistry()
	mock := &mockExecutor{}

	reg.Register("pod-kill", mock)

	got, err := reg.Get("pod-kill")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != mock {
		t.Fatal("expected same executor instance")
	}
}

func TestRegistryGetUnregistered(t *testing.T) {
	reg := executor.NewRegistry()

	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unregistered action type")
	}
}
