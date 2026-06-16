// Package netns resolves a target pod's network namespace and the host-side
// veth peer that carries its traffic.
//
// Why host-side: attaching tc/eBPF from the host netns to the pod's host-side
// veth peer needs only CAP_NET_ADMIN (+CAP_BPF for eBPF). It never enters the
// pod netns and never requires a privileged container, preserving the
// project's no-privileged thesis.
package netns

import (
	"context"
	"fmt"
)

type PodRef struct {
	Namespace   string
	Name        string
	ContainerID string
	NodeName    string
}

type Resolution struct {
	ContainerPID int
	NetnsPath    string
	// HostVethIfindex is the host-netns ifindex of the veth peer paired with
	// the pod's eth0. This is where tc/eBPF attaches.
	HostVethIfindex int
	HostVethName    string
	PodEth0Ifindex  int
	CgroupV2Path    string
}

// Resolver is the public API Wave 2 fault executors depend on.
type Resolver interface {
	Resolve(ctx context.Context, ref PodRef) (*Resolution, error)
	ResolveCgroupV2(ctx context.Context, ref PodRef) (string, error)
}

type containerInfo struct {
	pid          int
	cgroupV2Path string
}

// criClient is a seam so the runtime lookup can be faked without a real
// containerd socket.
type criClient interface {
	containerInfo(ctx context.Context, containerID string) (containerInfo, error)
}

type link struct {
	index   int
	name    string
	peerIdx int
}

// netlinkOps is a seam so the iflink->host-ifindex matching can be unit-tested
// with fakes instead of real sysfs reads and netlink syscalls.
type netlinkOps interface {
	podEth0(podProcRoot string) (link, error)
	hostLinkByIndex(index int) (link, error)
}

// matchHostVeth reads the pod eth0's peer index (the host-side veth ifindex)
// and confirms that host link is the reciprocal peer of the pod's eth0.
//
// The reciprocity check guards against a stale or mismatched netns handle
// pointing tc/eBPF at the wrong interface, which would silently fault an
// unrelated workload.
func matchHostVeth(nl netlinkOps, podProcRoot string) (hostLink link, podEth0Ifindex int, err error) {
	eth0, err := nl.podEth0(podProcRoot)
	if err != nil {
		return link{}, 0, fmt.Errorf("read pod eth0 via %s: %w", podProcRoot, err)
	}
	if eth0.peerIdx == 0 {
		return link{}, 0, fmt.Errorf("pod eth0 (ifindex %d) reports no veth peer; not a veth-backed pod", eth0.index)
	}

	host, err := nl.hostLinkByIndex(eth0.peerIdx)
	if err != nil {
		return link{}, 0, fmt.Errorf("host veth peer ifindex %d not found in host netns: %w", eth0.peerIdx, err)
	}
	if host.peerIdx != eth0.index {
		return link{}, 0, fmt.Errorf("veth pairing mismatch: host link %d peers ifindex %d, expected pod eth0 %d", host.index, host.peerIdx, eth0.index)
	}
	return host, eth0.index, nil
}
