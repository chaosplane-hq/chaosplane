package executor_test

import (
	"sync"
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

func TestRegistryList(t *testing.T) {
	reg := executor.NewRegistry()
	m1 := &mockExecutor{}
	m2 := &mockExecutor{}

	reg.Register("pod-kill", m1)
	reg.Register("network-delay", m2)

	list := reg.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 executors, got %d", len(list))
	}
	if list["pod-kill"] != m1 {
		t.Fatal("pod-kill executor mismatch")
	}
	if list["network-delay"] != m2 {
		t.Fatal("network-delay executor mismatch")
	}
}

func TestRegistryListReturnsCopy(t *testing.T) {
	reg := executor.NewRegistry()
	reg.Register("pod-kill", &mockExecutor{})

	list := reg.List()
	list["injected"] = &mockExecutor{}

	if reg.Count() != 1 {
		t.Fatal("mutating List() result should not affect registry")
	}
}

func TestRegistryCount(t *testing.T) {
	reg := executor.NewRegistry()
	if reg.Count() != 0 {
		t.Fatalf("expected 0, got %d", reg.Count())
	}

	reg.Register("pod-kill", &mockExecutor{})
	if reg.Count() != 1 {
		t.Fatalf("expected 1, got %d", reg.Count())
	}

	reg.Register("network-delay", &mockExecutor{})
	if reg.Count() != 2 {
		t.Fatalf("expected 2, got %d", reg.Count())
	}
}

func TestRegistryMustRegister(t *testing.T) {
	reg := executor.NewRegistry()
	reg.MustRegister("pod-kill", &mockExecutor{})

	got, err := reg.Get("pod-kill")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil executor")
	}
}

func TestRegistryMustRegisterPanicsOnDuplicate(t *testing.T) {
	reg := executor.NewRegistry()
	reg.MustRegister("pod-kill", &mockExecutor{})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate MustRegister")
		}
	}()

	reg.MustRegister("pod-kill", &mockExecutor{})
}

func TestRegistryConcurrentAccess(t *testing.T) {
	reg := executor.NewRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "action-" + string(rune('A'+n%26))
			reg.Register(key, &mockExecutor{})
			reg.Get(key)
			reg.List()
			reg.Count()
		}(i)
	}

	wg.Wait()

	if reg.Count() == 0 {
		t.Fatal("expected registered executors after concurrent access")
	}
}
