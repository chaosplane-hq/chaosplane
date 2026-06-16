//go:build linux

package netns

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vishvananda/netlink"
)

// New returns a Resolver backed by the host's CRI runtime and netlink. The
// criEndpoint is the containerd/CRI unix socket (e.g.
// unix:///run/containerd/containerd.sock).
func New(criEndpoint string) (Resolver, error) {
	cri, err := newCRIRuntimeClient(criEndpoint)
	if err != nil {
		return nil, err
	}
	return &resolver{cri: cri, nl: &netlinkImpl{}}, nil
}

type resolver struct {
	cri criClient
	nl  netlinkOps
}

func (r *resolver) Resolve(ctx context.Context, ref PodRef) (*Resolution, error) {
	if ref.ContainerID == "" {
		return nil, fmt.Errorf("netns resolve: container ID is required for pod %s/%s", ref.Namespace, ref.Name)
	}

	info, err := r.cri.containerInfo(ctx, ref.ContainerID)
	if err != nil {
		return nil, err
	}

	podProcRoot := podSysClassNet(info.pid)
	hostVeth, podEth0Ifindex, err := matchHostVeth(r.nl, podProcRoot)
	if err != nil {
		return nil, err
	}

	cgroupPath, err := readCgroupV2Path(info.pid)
	if err != nil {
		return nil, err
	}

	return &Resolution{
		ContainerPID:    info.pid,
		NetnsPath:       netnsPathForPID(info.pid),
		HostVethIfindex: hostVeth.index,
		HostVethName:    hostVeth.name,
		PodEth0Ifindex:  podEth0Ifindex,
		CgroupV2Path:    cgroupPath,
	}, nil
}

func (r *resolver) ResolveCgroupV2(ctx context.Context, ref PodRef) (string, error) {
	if ref.ContainerID == "" {
		return "", fmt.Errorf("netns resolve cgroup: container ID is required for pod %s/%s", ref.Namespace, ref.Name)
	}
	info, err := r.cri.containerInfo(ctx, ref.ContainerID)
	if err != nil {
		return "", err
	}
	return readCgroupV2Path(info.pid)
}

func readCgroupV2Path(pid int) (string, error) {
	contents, err := os.ReadFile(procCgroupPathForPID(pid))
	if err != nil {
		return "", fmt.Errorf("read cgroup for pid %d: %w", pid, err)
	}
	return parseCgroupV2Path(string(contents))
}

// netlinkImpl reads the pod's eth0 ifindex/iflink from sysfs through the
// container's proc root (no setns), then looks up the host-side peer with
// netlink in the host netns.
type netlinkImpl struct{}

func (netlinkImpl) podEth0(podProcRoot string) (link, error) {
	eth0Dir := filepath.Join(podProcRoot, "eth0")

	idxRaw, err := os.ReadFile(filepath.Join(eth0Dir, "ifindex"))
	if err != nil {
		return link{}, fmt.Errorf("read pod eth0 ifindex: %w", err)
	}
	index, err := parseIfindexFile(string(idxRaw))
	if err != nil {
		return link{}, fmt.Errorf("parse pod eth0 ifindex: %w", err)
	}

	peerRaw, err := os.ReadFile(filepath.Join(eth0Dir, "iflink"))
	if err != nil {
		return link{}, fmt.Errorf("read pod eth0 iflink: %w", err)
	}
	peerIdx, err := parseIfindexFile(string(peerRaw))
	if err != nil {
		return link{}, fmt.Errorf("parse pod eth0 iflink: %w", err)
	}

	return link{index: index, name: "eth0", peerIdx: peerIdx}, nil
}

func (netlinkImpl) hostLinkByIndex(index int) (link, error) {
	l, err := netlink.LinkByIndex(index)
	if err != nil {
		return link{}, fmt.Errorf("lookup host link ifindex %d: %w", index, err)
	}
	attrs := l.Attrs()
	return link{index: attrs.Index, name: attrs.Name, peerIdx: attrs.ParentIndex}, nil
}
