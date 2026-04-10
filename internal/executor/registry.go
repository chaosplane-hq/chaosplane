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
