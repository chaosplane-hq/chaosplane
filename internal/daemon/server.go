package daemon

import (
	"context"
	"fmt"
	"log"
	"time"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/google/uuid"
)

type Server struct {
	daemonv1.UnimplementedChaosDaemonServer
	store *ExecutionStore
}

func NewServer() *Server {
	return &Server{
		store: NewExecutionStore(),
	}
}

func (s *Server) ExecNetworkChaos(_ context.Context, req *daemonv1.NetworkChaosRequest) (*daemonv1.NetworkChaosResponse, error) {
	execID := uuid.New().String()
	s.store.Add(execID, ExecutionInfo{
		ID:           execID,
		ExperimentID: req.GetExperimentId(),
		Type:         "network",
		Status:       "running",
		StartTime:    time.Now(),
		Parameters:   req.GetParameters(),
	})
	log.Printf("exec network chaos: id=%s experiment=%s action=%s iface=%s", execID, req.GetExperimentId(), req.GetAction(), req.GetTargetIface())
	return &daemonv1.NetworkChaosResponse{
		Success:     true,
		Message:     fmt.Sprintf("network chaos applied: %s", req.GetAction()),
		ExecutionId: execID,
	}, nil
}

func (s *Server) ExecStressChaos(_ context.Context, req *daemonv1.StressChaosRequest) (*daemonv1.StressChaosResponse, error) {
	execID := uuid.New().String()
	s.store.Add(execID, ExecutionInfo{
		ID:           execID,
		ExperimentID: req.GetExperimentId(),
		Type:         "stress",
		Status:       "running",
		StartTime:    time.Now(),
		Parameters:   req.GetParameters(),
	})
	log.Printf("exec stress chaos: id=%s experiment=%s stressor=%s", execID, req.GetExperimentId(), req.GetStressorType())
	return &daemonv1.StressChaosResponse{
		Success:     true,
		Message:     fmt.Sprintf("stress chaos applied: %s", req.GetStressorType()),
		ExecutionId: execID,
	}, nil
}

func (s *Server) ExecDNSChaos(_ context.Context, req *daemonv1.DNSChaosRequest) (*daemonv1.DNSChaosResponse, error) {
	execID := uuid.New().String()
	s.store.Add(execID, ExecutionInfo{
		ID:           execID,
		ExperimentID: req.GetExperimentId(),
		Type:         "dns",
		Status:       "running",
		StartTime:    time.Now(),
		Parameters:   req.GetParameters(),
	})
	log.Printf("exec dns chaos: id=%s experiment=%s action=%s", execID, req.GetExperimentId(), req.GetAction())
	return &daemonv1.DNSChaosResponse{
		Success:     true,
		Message:     fmt.Sprintf("dns chaos applied: %s", req.GetAction()),
		ExecutionId: execID,
	}, nil
}

func (s *Server) ExecHTTPChaos(_ context.Context, req *daemonv1.HTTPChaosRequest) (*daemonv1.HTTPChaosResponse, error) {
	execID := uuid.New().String()
	s.store.Add(execID, ExecutionInfo{
		ID:           execID,
		ExperimentID: req.GetExperimentId(),
		Type:         "http",
		Status:       "running",
		StartTime:    time.Now(),
		Parameters:   req.GetParameters(),
	})
	log.Printf("exec http chaos: id=%s experiment=%s action=%s port=%d", execID, req.GetExperimentId(), req.GetAction(), req.GetPort())
	return &daemonv1.HTTPChaosResponse{
		Success:     true,
		Message:     fmt.Sprintf("http chaos applied: %s", req.GetAction()),
		ExecutionId: execID,
	}, nil
}

func (s *Server) ExecNodeChaos(_ context.Context, req *daemonv1.NodeChaosRequest) (*daemonv1.NodeChaosResponse, error) {
	execID := uuid.New().String()
	s.store.Add(execID, ExecutionInfo{
		ID:           execID,
		ExperimentID: req.GetExperimentId(),
		Type:         "node",
		Status:       "running",
		StartTime:    time.Now(),
		Parameters:   req.GetParameters(),
	})
	log.Printf("exec node chaos: id=%s experiment=%s action=%s", execID, req.GetExperimentId(), req.GetAction())
	return &daemonv1.NodeChaosResponse{
		Success:     true,
		Message:     fmt.Sprintf("node chaos applied: %s", req.GetAction()),
		ExecutionId: execID,
	}, nil
}

func (s *Server) CancelChaos(_ context.Context, req *daemonv1.CancelRequest) (*daemonv1.CancelResponse, error) {
	execID := req.GetExecutionId()
	if _, ok := s.store.Get(execID); !ok {
		return &daemonv1.CancelResponse{
			Success: false,
			Message: fmt.Sprintf("execution %s not found", execID),
		}, nil
	}
	s.store.Remove(execID)
	log.Printf("cancel chaos: id=%s", execID)
	return &daemonv1.CancelResponse{
		Success: true,
		Message: fmt.Sprintf("execution %s cancelled", execID),
	}, nil
}

func (s *Server) GetChaosStatus(_ context.Context, _ *daemonv1.StatusRequest) (*daemonv1.StatusResponse, error) {
	executions := s.store.List()
	statuses := make([]*daemonv1.ExecutionStatus, 0, len(executions))
	for _, e := range executions {
		statuses = append(statuses, &daemonv1.ExecutionStatus{
			ExecutionId:  e.ID,
			ExperimentId: e.ExperimentID,
			Type:         e.Type,
			Status:       e.Status,
			StartTime:    e.StartTime.Format(time.RFC3339),
		})
	}
	return &daemonv1.StatusResponse{Executions: statuses}, nil
}
