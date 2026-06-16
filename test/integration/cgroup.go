//go:build integration

package integration

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CgroupStats reports resource-pressure readings sampled from inside a pod.
// Values are read from the container's own cgroup so they reflect what a stress
// fault actually imposed on the target, independent of the kubelet's view.
type CgroupStats struct {
	// CPUUsageUsec is cumulative on-CPU time in microseconds (cgroup cpu.stat).
	CPUUsageUsec uint64
	// MemoryCurrentBytes is the current memory charge (cgroup memory.current).
	MemoryCurrentBytes uint64
}

// ReadCgroupStats samples cpu.stat and memory.current from the pod's cgroup.
// It targets the unified (cgroup v2) hierarchy at /sys/fs/cgroup, which is what
// kind nodes expose. CPU-stress tests sample twice and assert the usec delta;
// memory-stress tests assert MemoryCurrentBytes rises toward the stress size.
func (h *Harness) ReadCgroupStats(ctx context.Context, namespace, pod string) (CgroupStats, error) {
	var stats CgroupStats

	cpu, err := h.Exec(ctx, namespace, pod, "cat", "/sys/fs/cgroup/cpu.stat")
	if err != nil {
		return stats, fmt.Errorf("read cpu.stat: %w", err)
	}
	stats.CPUUsageUsec = parseCPUUsage(cpu.Stdout)

	mem, err := h.Exec(ctx, namespace, pod, "cat", "/sys/fs/cgroup/memory.current")
	if err != nil {
		return stats, fmt.Errorf("read memory.current: %w", err)
	}
	if v, perr := strconv.ParseUint(trimmed(mem.Stdout), 10, 64); perr == nil {
		stats.MemoryCurrentBytes = v
	}
	return stats, nil
}

func parseCPUUsage(out string) uint64 {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "usage_usec" {
			v, err := strconv.ParseUint(fields[1], 10, 64)
			if err == nil {
				return v
			}
		}
	}
	return 0
}

// CPUBusyFraction samples cgroup CPU usage across window and returns on-CPU
// time as a fraction of one core. A value near or above 1.0 indicates a
// CPU-stress fault is saturating at least a full core.
func (h *Harness) CPUBusyFraction(ctx context.Context, namespace, pod string, window time.Duration) (float64, error) {
	first, err := h.ReadCgroupStats(ctx, namespace, pod)
	if err != nil {
		return 0, err
	}
	time.Sleep(window)
	second, err := h.ReadCgroupStats(ctx, namespace, pod)
	if err != nil {
		return 0, err
	}

	busyUsec := float64(second.CPUUsageUsec - first.CPUUsageUsec)
	windowUsec := float64(window.Microseconds())
	if windowUsec == 0 {
		return 0, fmt.Errorf("zero sampling window")
	}
	return busyUsec / windowUsec, nil
}
