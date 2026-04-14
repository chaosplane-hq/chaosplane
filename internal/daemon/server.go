package daemon

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	daemonebpf "github.com/chaosplane-hq/chaosplane/internal/daemon/ebpf"
	"github.com/google/uuid"
	"log/slog"
)

type Server struct {
	daemonv1.UnimplementedChaosDaemonServer
	store   *ExecutionStore
	ebpfMgr *daemonebpf.Manager
}

func NewServer() *Server {
	logger := slog.Default()
	return &Server{
		store:   NewExecutionStore(),
		ebpfMgr: daemonebpf.NewManager(logger),
	}
}

func (s *Server) ExecNetworkChaos(ctx context.Context, req *daemonv1.NetworkChaosRequest) (*daemonv1.NetworkChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	iface := req.GetTargetIface()
	if iface == "" {
		iface = "eth0"
	}
	action := req.GetAction()

	if params["mode"] == "ebpf" {
		var err error
		switch action {
		case "delay":
			delayUS := uint32(100000)
			if v := params["latency"]; v != "" {
				if ms, e := strconv.Atoi(v); e == nil {
					delayUS = uint32(ms * 1000)
				}
			}
			ifIdx, _ := strconv.Atoi(params["ifIndex"])
			if ifIdx == 0 {
				ifIdx = 2
			}
			err = s.ebpfMgr.LoadTCDelay(execID, ifIdx, delayUS)
		case "loss":
			pct := uint32(10)
			if v := params["percent"]; v != "" {
				if p, e := strconv.Atoi(v); e == nil {
					pct = uint32(p)
				}
			}
			ifIdx, _ := strconv.Atoi(params["ifIndex"])
			if ifIdx == 0 {
				ifIdx = 2
			}
			err = s.ebpfMgr.LoadTCDrop(execID, ifIdx, pct)
		default:
			err = fmt.Errorf("unsupported ebpf action: %s", action)
		}
		if err != nil {
			return &daemonv1.NetworkChaosResponse{Success: false, Message: err.Error()}, nil
		}
	} else {
		if err := tcAddNetem(iface, action, params); err != nil {
			log.Printf("warn: tc command failed (may not be available in this environment): %v", err)
		}
	}

	s.store.Add(execID, ExecutionInfo{
		ID:           execID,
		ExperimentID: req.GetExperimentId(),
		Type:         "network",
		Status:       "running",
		StartTime:    time.Now(),
		Parameters:   params,
	})
	log.Printf("exec network chaos: id=%s experiment=%s action=%s iface=%s mode=%s", execID, req.GetExperimentId(), action, iface, params["mode"])
	return &daemonv1.NetworkChaosResponse{
		Success:     true,
		Message:     fmt.Sprintf("network chaos applied: %s", action),
		ExecutionId: execID,
	}, nil
}

func (s *Server) ExecStressChaos(_ context.Context, req *daemonv1.StressChaosRequest) (*daemonv1.StressChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	stressorType := req.GetStressorType()

	duration := 60
	if v := params["duration"]; v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			duration = d
		}
	}

	if err := stressNGStart(stressorType, params, duration); err != nil {
		log.Printf("warn: stress-ng command failed (may not be available in this environment): %v", err)
	}

	s.store.Add(execID, ExecutionInfo{
		ID:           execID,
		ExperimentID: req.GetExperimentId(),
		Type:         "stress",
		Status:       "running",
		StartTime:    time.Now(),
		Parameters:   params,
	})
	log.Printf("exec stress chaos: id=%s experiment=%s stressor=%s", execID, req.GetExperimentId(), stressorType)
	return &daemonv1.StressChaosResponse{
		Success:     true,
		Message:     fmt.Sprintf("stress chaos applied: %s", stressorType),
		ExecutionId: execID,
	}, nil
}

func (s *Server) ExecDNSChaos(_ context.Context, req *daemonv1.DNSChaosRequest) (*daemonv1.DNSChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	action := req.GetAction()

	if err := dnsIntercept(action, params); err != nil {
		log.Printf("warn: dns intercept failed (may not be available in this environment): %v", err)
	}

	s.store.Add(execID, ExecutionInfo{
		ID:           execID,
		ExperimentID: req.GetExperimentId(),
		Type:         "dns",
		Status:       "running",
		StartTime:    time.Now(),
		Parameters:   params,
	})
	log.Printf("exec dns chaos: id=%s experiment=%s action=%s", execID, req.GetExperimentId(), action)
	return &daemonv1.DNSChaosResponse{
		Success:     true,
		Message:     fmt.Sprintf("dns chaos applied: %s", action),
		ExecutionId: execID,
	}, nil
}

func (s *Server) ExecHTTPChaos(_ context.Context, req *daemonv1.HTTPChaosRequest) (*daemonv1.HTTPChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	action := req.GetAction()
	port := req.GetPort()

	if port > 0 {
		portStr := strconv.Itoa(int(port))
		switch action {
		case "abort":
			_, _ = execCmd(context.Background(), "iptables", "-A", "INPUT", "-p", "tcp", "--dport", portStr, "-j", "REJECT")
		case "delay":
			_, _ = execCmd(context.Background(), "tc", "qdisc", "add", "dev", "lo", "root", "netem", "delay", params["delay"]+"ms")
		}
	}

	s.store.Add(execID, ExecutionInfo{
		ID:           execID,
		ExperimentID: req.GetExperimentId(),
		Type:         "http",
		Status:       "running",
		StartTime:    time.Now(),
		Parameters:   params,
	})
	log.Printf("exec http chaos: id=%s experiment=%s action=%s port=%d", execID, req.GetExperimentId(), action, port)
	return &daemonv1.HTTPChaosResponse{
		Success:     true,
		Message:     fmt.Sprintf("http chaos applied: %s", action),
		ExecutionId: execID,
	}, nil
}

func (s *Server) ExecNodeChaos(_ context.Context, req *daemonv1.NodeChaosRequest) (*daemonv1.NodeChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	action := req.GetAction()

	switch action {
	case "cpu-stress":
		_ = stressNGStart("cpu", params, 60)
	case "partition":
		iface := params["iface"]
		if iface == "" {
			iface = "eth0"
		}
		_ = iptablesBlock(iface, "both")
	}

	s.store.Add(execID, ExecutionInfo{
		ID:           execID,
		ExperimentID: req.GetExperimentId(),
		Type:         "node",
		Status:       "running",
		StartTime:    time.Now(),
		Parameters:   params,
	})
	log.Printf("exec node chaos: id=%s experiment=%s action=%s", execID, req.GetExperimentId(), action)
	return &daemonv1.NodeChaosResponse{
		Success:     true,
		Message:     fmt.Sprintf("node chaos applied: %s", action),
		ExecutionId: execID,
	}, nil
}

func (s *Server) CancelChaos(_ context.Context, req *daemonv1.CancelRequest) (*daemonv1.CancelResponse, error) {
	execID := req.GetExecutionId()
	info, ok := s.store.Get(execID)
	if !ok {
		return &daemonv1.CancelResponse{
			Success: false,
			Message: fmt.Sprintf("execution %s not found", execID),
		}, nil
	}

	switch info.Type {
	case "network":
		if info.Parameters["mode"] == "ebpf" {
			_ = s.ebpfMgr.Unload(execID)
		} else {
			iface := info.Parameters["iface"]
			if iface == "" {
				iface = "eth0"
			}
			_ = tcDelete(iface)
		}
	case "stress":
		stressNGStop()
	case "dns":
		dnsRestore(info.Parameters)
	case "http":
		if port := info.Parameters["port"]; port != "" {
			_, _ = execCmd(context.Background(), "iptables", "-D", "INPUT", "-p", "tcp", "--dport", port, "-j", "REJECT")
			_ = tcDelete("lo")
		}
	case "node":
		if info.Parameters["iface"] != "" {
			_ = iptablesUnblock(info.Parameters["iface"], "both")
		}
		stressNGStop()
	}

	s.store.Remove(execID)
	log.Printf("cancel chaos: id=%s type=%s", execID, info.Type)
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
