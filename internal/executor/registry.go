package executor

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	executors map[string]Executor
}

func NewRegistry() *Registry {
	return &Registry{
		executors: make(map[string]Executor),
	}
}

func (r *Registry) Register(actionType string, exec Executor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[actionType] = exec
}

func (r *Registry) Get(actionType string) (Executor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	exec, ok := r.executors[actionType]
	if !ok {
		return nil, fmt.Errorf("no executor registered for action type %q", actionType)
	}
	return exec, nil
}

// MustRegister registers an executor and panics if the action type is already registered.
func (r *Registry) MustRegister(actionType string, exec Executor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.executors[actionType]; exists {
		panic(fmt.Sprintf("executor already registered for action type %q", actionType))
	}
	r.executors[actionType] = exec
}

// List returns a copy of all registered executors.
func (r *Registry) List() map[string]Executor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copy := make(map[string]Executor, len(r.executors))
	for k, v := range r.executors {
		copy[k] = v
	}
	return copy
}

// Count returns the number of registered executors.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.executors)
}
