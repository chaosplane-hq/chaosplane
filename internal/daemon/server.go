package daemon

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	daemonebpf "github.com/chaosplane-hq/chaosplane/internal/daemon/ebpf"
	"github.com/chaosplane-hq/chaosplane/internal/daemon/netns"
	"github.com/google/uuid"
	"log/slog"
)

type Server struct {
	daemonv1.UnimplementedChaosDaemonServer
	store    *ExecutionStore
	ebpfMgr  *daemonebpf.Manager
	sys      *sysOps
	resolver netns.Resolver
}

func NewServer() *Server {
	logger := slog.Default()
	return &Server{
		store:   NewExecutionStore(),
		ebpfMgr: daemonebpf.NewManager(logger),
		sys:     newSysOps(execRunner{}),
	}
}

// SetResolver wires the pod netns/host-veth resolver. It is configured once the
// daemon has a CRI socket (T19); until then network chaos targeting a pod
// honestly reports failure rather than attaching to the wrong interface.
func (s *Server) SetResolver(r netns.Resolver) {
	s.resolver = r
}

func newServerWithRunner(r commandRunner) *Server {
	logger := slog.Default()
	return &Server{
		store:   NewExecutionStore(),
		ebpfMgr: daemonebpf.NewManager(logger),
		sys:     newSysOps(r),
	}
}

func (s *Server) ExecNetworkChaos(ctx context.Context, req *daemonv1.NetworkChaosRequest) (*daemonv1.NetworkChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	action := req.GetAction()

	iface, ifIndex, errResp := s.resolveTarget(ctx, req)
	if errResp != nil {
		return errResp, nil
	}

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
			err = s.ebpfMgr.LoadTCDelay(execID, ifIndex, delayUS)
		case "loss":
			pct := uint32(10)
			if v := params["percent"]; v != "" {
				if p, e := strconv.Atoi(v); e == nil {
					pct = uint32(p)
				}
			}
			err = s.ebpfMgr.LoadTCDrop(execID, ifIndex, pct)
		default:
			err = fmt.Errorf("unsupported ebpf action: %s", action)
		}
		if err != nil {
			return &daemonv1.NetworkChaosResponse{Success: false, Applied: false, Message: err.Error()}, nil
		}
	} else {
		if err := s.sys.tcAddNetem(iface, action, params); err != nil {
			return &daemonv1.NetworkChaosResponse{
				Success: false,
				Applied: false,
				Message: fmt.Sprintf("network chaos %q failed on iface %s: %v", action, iface, err),
			}, nil
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
		Applied:     true,
		Message:     fmt.Sprintf("network chaos applied: %s", action),
		ExecutionId: execID,
	}, nil
}

// resolveTarget determines the host-side veth interface name and ifindex for a
// network chaos request. When the request carries a pod identity, it resolves
// the real host-side veth peer server-side (no privileged container, no setns
// into the pod). A pod target that cannot be resolved is reported as a failure
// rather than silently faulting the daemon's own interface.
func (s *Server) resolveTarget(ctx context.Context, req *daemonv1.NetworkChaosRequest) (iface string, ifIndex int, errResp *daemonv1.NetworkChaosResponse) {
	if req.GetPodName() == "" && req.GetContainerId() == "" {
		iface = req.GetTargetIface()
		if iface == "" {
			iface = "eth0"
		}
		ifIndex, _ = strconv.Atoi(req.GetParameters()["ifIndex"])
		return iface, ifIndex, nil
	}

	if s.resolver == nil {
		return "", 0, &daemonv1.NetworkChaosResponse{
			Success: false,
			Applied: false,
			Message: fmt.Sprintf("cannot resolve host-side veth for pod %s/%s: no CRI resolver configured on this daemon", req.GetNamespace(), req.GetPodName()),
		}
	}

	res, err := s.resolver.Resolve(ctx, netns.PodRef{
		Namespace:   req.GetNamespace(),
		Name:        req.GetPodName(),
		ContainerID: req.GetContainerId(),
		NodeName:    req.GetNodeName(),
	})
	if err != nil {
		return "", 0, &daemonv1.NetworkChaosResponse{
			Success: false,
			Applied: false,
			Message: fmt.Sprintf("resolve host-side veth for pod %s/%s: %v", req.GetNamespace(), req.GetPodName(), err),
		}
	}
	return res.HostVethName, res.HostVethIfindex, nil
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

	if err := s.sys.stressNGStart(stressorType, params, duration); err != nil {
		return &daemonv1.StressChaosResponse{
			Success: false,
			Applied: false,
			Message: fmt.Sprintf("stress chaos %q failed: %v", stressorType, err),
		}, nil
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
		Applied:     true,
		Message:     fmt.Sprintf("stress chaos applied: %s", stressorType),
		ExecutionId: execID,
	}, nil
}

func (s *Server) ExecDNSChaos(_ context.Context, req *daemonv1.DNSChaosRequest) (*daemonv1.DNSChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	action := req.GetAction()

	if err := s.sys.dnsIntercept(action, params); err != nil {
		return &daemonv1.DNSChaosResponse{
			Success: false,
			Applied: false,
			Message: fmt.Sprintf("dns chaos %q failed: %v", action, err),
		}, nil
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
		Applied:     true,
		Message:     fmt.Sprintf("dns chaos applied: %s", action),
		ExecutionId: execID,
	}, nil
}

func (s *Server) ExecHTTPChaos(_ context.Context, req *daemonv1.HTTPChaosRequest) (*daemonv1.HTTPChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	action := req.GetAction()
	port := req.GetPort()

	if port <= 0 {
		return &daemonv1.HTTPChaosResponse{
			Success: false,
			Applied: false,
			Message: "http chaos requires a positive port",
		}, nil
	}

	portStr := strconv.Itoa(int(port))
	var err error
	switch action {
	case "abort":
		_, err = s.sys.runner.Run(context.Background(), "iptables", "-A", "INPUT", "-p", "tcp", "--dport", portStr, "-j", "REJECT")
	case "delay":
		_, err = s.sys.runner.Run(context.Background(), "tc", "qdisc", "add", "dev", "lo", "root", "netem", "delay", params["delay"]+"ms")
	default:
		return &daemonv1.HTTPChaosResponse{
			Success: false,
			Applied: false,
			Message: fmt.Sprintf("unsupported http chaos action: %s", action),
		}, nil
	}
	if err != nil {
		return &daemonv1.HTTPChaosResponse{
			Success: false,
			Applied: false,
			Message: fmt.Sprintf("http chaos %q failed on port %d: %v", action, port, err),
		}, nil
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
		Applied:     true,
		Message:     fmt.Sprintf("http chaos applied: %s", action),
		ExecutionId: execID,
	}, nil
}

func (s *Server) ExecNodeChaos(_ context.Context, req *daemonv1.NodeChaosRequest) (*daemonv1.NodeChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	action := req.GetAction()

	var err error
	switch action {
	case "cpu-stress":
		err = s.sys.stressNGStart("cpu", params, 60)
	case "partition":
		iface := params["iface"]
		if iface == "" {
			iface = "eth0"
		}
		err = s.sys.iptablesBlock(iface, "both")
	default:
		return &daemonv1.NodeChaosResponse{
			Success: false,
			Applied: false,
			Message: fmt.Sprintf("unsupported node chaos action: %s", action),
		}, nil
	}
	if err != nil {
		return &daemonv1.NodeChaosResponse{
			Success: false,
			Applied: false,
			Message: fmt.Sprintf("node chaos %q failed: %v", action, err),
		}, nil
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
		Applied:     true,
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
			_ = s.sys.tcDelete(iface)
		}
	case "stress":
		s.sys.stressNGStop()
	case "dns":
		s.sys.dnsRestore(info.Parameters)
	case "http":
		if port := info.Parameters["port"]; port != "" {
			_, _ = s.sys.runner.Run(context.Background(), "iptables", "-D", "INPUT", "-p", "tcp", "--dport", port, "-j", "REJECT")
			_ = s.sys.tcDelete("lo")
		}
	case "node":
		if info.Parameters["iface"] != "" {
			_ = s.sys.iptablesUnblock(info.Parameters["iface"], "both")
		}
		s.sys.stressNGStop()
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
