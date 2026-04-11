package extension

import (
	"fmt"
	"sync"
)

// Registry manages the lifecycle of ChaosPlugin instances.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]ChaosPlugin
}

func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]ChaosPlugin),
	}
}

func (r *Registry) Register(p ChaosPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := p.Name()
	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin %q already registered", name)
	}
	r.plugins[name] = p
	return nil
}

func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; !exists {
		return fmt.Errorf("plugin %q not found", name)
	}
	delete(r.plugins, name)
	return nil
}

func (r *Registry) Get(name string) (ChaosPlugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin %q not found", name)
	}
	return p, nil
}

func (r *Registry) List() []ChaosPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ChaosPlugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		result = append(result, p)
	}
	return result
}
