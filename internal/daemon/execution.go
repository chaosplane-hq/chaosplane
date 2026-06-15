package daemon

import (
	"sync"
	"time"
)

type ExecutionInfo struct {
	ID           string
	ExperimentID string
	Type         string
	Status       string
	StartTime    time.Time
	Parameters   map[string]string
}

type ExecutionStore struct {
	mu         sync.RWMutex
	executions map[string]ExecutionInfo
}

func NewExecutionStore() *ExecutionStore {
	return &ExecutionStore{
		executions: make(map[string]ExecutionInfo),
	}
}

func (s *ExecutionStore) Add(id string, info ExecutionInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions[id] = info
}

func (s *ExecutionStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.executions, id)
}

func (s *ExecutionStore) Get(id string) (ExecutionInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info, ok := s.executions[id]
	return info, ok
}

func (s *ExecutionStore) List() []ExecutionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ExecutionInfo, 0, len(s.executions))
	for _, info := range s.executions {
		result = append(result, info)
	}
	return result
}
