package daemon

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
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
	if params == nil {
		params = map[string]string{}
	}
	action := req.GetAction()

	iface, ifIndex, errResp := s.resolveTarget(ctx, req)
	if errResp != nil {
		return errResp, nil
	}

	// Partition is an iptables drop on the host-side veth, not a netem qdisc:
	// it blocks reachability to/from a CIDR per direction and must be undone by
	// deleting the exact rules, so it has its own dispatch and cleanup path.
	if action == "partition" {
		if err := s.sys.partition(iface, params["direction"], params["target_cidr"]); err != nil {
			return &daemonv1.NetworkChaosResponse{
				Success: false,
				Applied: false,
				Message: fmt.Sprintf("network partition failed on iface %s: %v", iface, err),
			}, nil
		}
		params["iface"] = iface
		params["datapath"] = "iptables"
		s.store.Add(execID, ExecutionInfo{
			ID:           execID,
			ExperimentID: req.GetExperimentId(),
			Type:         "network",
			Status:       "running",
			StartTime:    time.Now(),
			Parameters:   params,
		})
		log.Printf("exec network chaos: id=%s experiment=%s action=partition iface=%s cidr=%s dir=%s", execID, req.GetExperimentId(), iface, params["target_cidr"], params["direction"])
		return &daemonv1.NetworkChaosResponse{
			Success:     true,
			Applied:     true,
			Message:     "network chaos applied: partition",
			ExecutionId: execID,
		}, nil
	}

	// Delay is always a tc-netem fault: a TC/eBPF classifier cannot sleep, so
	// there is no eBPF datapath that can actually delay a packet. Only loss has
	// a real eBPF implementation; everything else (and delay specifically) runs
	// through netem on the resolved host-side veth.
	if params["mode"] == "ebpf" && action == "loss" {
		pct := uint32(10)
		if v := params["percent"]; v != "" {
			if p, e := strconv.Atoi(strings.TrimSuffix(v, "%")); e == nil {
				pct = uint32(p)
			}
		}
		if err := s.ebpfMgr.LoadTCDrop(execID, ifIndex, pct); err != nil {
			// eBPF attach can fail on kernels without TCX or without CAP_BPF;
			// fall back to netem loss so the fault still applies, recording
			// which datapath actually took effect.
			if ferr := s.sys.tcAddNetem(iface, "loss", params); ferr != nil {
				return &daemonv1.NetworkChaosResponse{
					Success: false,
					Applied: false,
					Message: fmt.Sprintf("ebpf loss attach failed (%v) and netem fallback failed (%v) on iface %s", err, ferr, iface),
				}, nil
			}
			params["datapath"] = "netem-fallback"
		} else {
			params["datapath"] = "ebpf"
		}
	} else {
		if err := s.sys.tcAddNetem(iface, action, params); err != nil {
			return &daemonv1.NetworkChaosResponse{
				Success: false,
				Applied: false,
				Message: fmt.Sprintf("network chaos %q failed on iface %s: %v", action, iface, err),
			}, nil
		}
		params["datapath"] = "netem"
	}

	// Record the resolved iface so cleanup deletes the qdisc on the same
	// host-side veth, not a guessed default.
	params["iface"] = iface

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

// podTarget carries the host-side facts the DNS/HTTP/stress faults need to act
// on a specific pod instead of the daemon itself.
type podTarget struct {
	scoped       bool // true when the request named a pod and resolution succeeded
	hostVeth     string
	cgroupV2Path string
}

// resolvePodTarget resolves the host-side veth and cgroup for a pod identified
// by params (podName/podNamespace/containerId/nodeName). The DNS/HTTP/stress
// RPCs predate the NetworkChaosRequest pod-identity proto fields, so they carry
// identity in the parameter map; this keeps the wire format stable.
//
// When no pod identity is present, scoped=false is returned with no error so
// the caller falls back to host scope. When identity IS present but no resolver
// is configured (CRI socket not wired yet) or resolution fails, an error is
// returned so the daemon reports failure instead of faulting itself.
func (s *Server) resolvePodTarget(ctx context.Context, params map[string]string) (podTarget, error) {
	podName := params["podName"]
	containerID := params["containerId"]
	if podName == "" && containerID == "" {
		return podTarget{}, nil
	}
	if s.resolver == nil {
		return podTarget{}, fmt.Errorf("cannot resolve pod %s/%s: no CRI resolver configured on this daemon", params["podNamespace"], podName)
	}
	res, err := s.resolver.Resolve(ctx, netns.PodRef{
		Namespace:   params["podNamespace"],
		Name:        podName,
		ContainerID: containerID,
		NodeName:    params["nodeName"],
	})
	if err != nil {
		return podTarget{}, fmt.Errorf("resolve pod %s/%s: %w", params["podNamespace"], podName, err)
	}
	return podTarget{scoped: true, hostVeth: res.HostVethName, cgroupV2Path: res.CgroupV2Path}, nil
}

func (s *Server) ExecStressChaos(ctx context.Context, req *daemonv1.StressChaosRequest) (*daemonv1.StressChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	if params == nil {
		params = map[string]string{}
	}
	stressorType := req.GetStressorType()

	duration := 60
	if v := params["duration"]; v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			duration = d
		}
	}

	target, err := s.resolvePodTarget(ctx, params)
	if err != nil {
		return &daemonv1.StressChaosResponse{Success: false, Applied: false, Message: err.Error()}, nil
	}
	// A pod target must be cgroup-scoped or it would stress the whole node; a
	// resolved pod with an empty cgroup path is a resolver bug, reported here
	// rather than silently stressing the host.
	if target.scoped && target.cgroupV2Path == "" {
		return &daemonv1.StressChaosResponse{
			Success: false,
			Applied: false,
			Message: fmt.Sprintf("stress chaos %q: pod resolved but cgroup v2 path is empty; refusing to stress the whole node", stressorType),
		}, nil
	}

	if err := s.sys.stressNGStart(stressorType, params, duration, target.cgroupV2Path); err != nil {
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

func (s *Server) ExecDNSChaos(ctx context.Context, req *daemonv1.DNSChaosRequest) (*daemonv1.DNSChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	if params == nil {
		params = map[string]string{}
	}
	action := req.GetAction()

	target, err := s.resolvePodTarget(ctx, params)
	if err != nil {
		return &daemonv1.DNSChaosResponse{Success: false, Applied: false, Message: err.Error()}, nil
	}
	params["iface"] = target.hostVeth

	if err := s.sys.dnsIntercept(action, target, params); err != nil {
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

func (s *Server) ExecHTTPChaos(ctx context.Context, req *daemonv1.HTTPChaosRequest) (*daemonv1.HTTPChaosResponse, error) {
	execID := uuid.New().String()
	params := req.GetParameters()
	if params == nil {
		params = map[string]string{}
	}
	action := req.GetAction()
	port := req.GetPort()

	if port <= 0 {
		return &daemonv1.HTTPChaosResponse{
			Success: false,
			Applied: false,
			Message: "http chaos requires a positive port",
		}, nil
	}

	target, err := s.resolvePodTarget(ctx, params)
	if err != nil {
		return &daemonv1.HTTPChaosResponse{Success: false, Applied: false, Message: err.Error()}, nil
	}
	params["iface"] = target.hostVeth
	params["port"] = strconv.Itoa(int(port))

	switch action {
	case "abort":
		err = s.sys.httpAbort(target, int(port))
	case "delay":
		err = s.sys.httpDelay(target, params["delay"])
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
	params["action"] = action

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
		err = s.sys.stressNGStart("cpu", params, 60, "")
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
		iface := info.Parameters["iface"]
		if iface == "" {
			iface = "eth0"
		}
		switch info.Parameters["datapath"] {
		case "ebpf":
			_ = s.ebpfMgr.Unload(execID)
		case "iptables":
			s.sys.partitionRestore(iface, info.Parameters["direction"], info.Parameters["target_cidr"])
		default:
			_ = s.sys.tcDelete(iface)
		}
	case "stress":
		s.sys.stressNGStop()
	case "dns":
		s.sys.dnsRestore(info.Parameters)
	case "http":
		s.sys.httpRestore(info.Parameters)
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
