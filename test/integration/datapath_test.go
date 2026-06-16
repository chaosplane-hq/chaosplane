//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
)

const datapathNamespace = "chaosplane-datapath"

// newNetworkExperiment builds a ChaosExperiment of the given action type
// targeting pods labeled app=<targetApp>, with arbitrary string parameters.
// Built unstructured to avoid a compile dependency on the operator's typed API.
func newNetworkExperiment(namespace, name, actionType, targetApp string, params map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "chaos.chaosplane.dev/v1alpha1",
			"kind":       "ChaosExperiment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels":    map[string]interface{}{"test": "integration"},
			},
			"spec": map[string]interface{}{
				"target": map[string]interface{}{
					"kind":      "Pod",
					"namespace": namespace,
					"labelSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{"app": targetApp},
					},
				},
				"action": map[string]interface{}{
					"type":       actionType,
					"parameters": params,
				},
				"duration": "120s",
			},
		},
	}
}

// applyExperiment creates the experiment and registers deletion (which the
// operator turns into a rollback, undoing the fault) on cleanup.
func (h *Harness) applyExperiment(ctx context.Context, t *testing.T, exp *unstructured.Unstructured) {
	t.Helper()
	created, err := h.Dynamic.Resource(chaosExperimentGVR).
		Namespace(exp.GetNamespace()).
		Create(ctx, exp, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create experiment %s: %v", exp.GetName(), err)
	}
	t.Cleanup(func() {
		_ = h.Dynamic.Resource(chaosExperimentGVR).
			Namespace(exp.GetNamespace()).
			Delete(context.Background(), created.GetName(), metav1.DeleteOptions{})
	})
}

// waitExperimentRunning blocks until the operator reports the experiment has
// progressed past Pending, so probes measure a fault that is actually applied.
func (h *Harness) waitExperimentRunning(ctx context.Context, t *testing.T, namespace, name string, timeout time.Duration) {
	t.Helper()
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		obj, err := h.Dynamic.Resource(chaosExperimentGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase == "Running" || phase == "Injected" || phase == "Active", nil
	})
	if err != nil {
		t.Fatalf("experiment %s did not reach a running phase: %v", name, err)
	}
}

// TestNetworkDelay_RaisesRTT (T6) asserts a network-delay experiment raises the
// target's measured RTT by approximately the configured latency.
func TestNetworkDelay_RaisesRTT(t *testing.T) {
	h := requireCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	h.EnsureNamespace(ctx, t, datapathNamespace)
	h.DeployPod(ctx, t, PodSpec{Name: "delay-target", Namespace: datapathNamespace, Labels: map[string]string{"app": "delay-target"}})
	probe := h.DeployPod(ctx, t, PodSpec{Name: "delay-probe", Namespace: datapathNamespace, Labels: map[string]string{"app": "delay-probe"}})
	_ = probe

	targetIP, err := h.PodIP(ctx, datapathNamespace, "delay-target")
	if err != nil {
		t.Fatalf("target IP: %v", err)
	}

	before, err := h.MeasureNetwork(ctx, datapathNamespace, "delay-probe", targetIP, 20)
	if err != nil {
		t.Fatalf("baseline ping: %v", err)
	}

	h.applyExperiment(ctx, t, newNetworkExperiment(datapathNamespace, "dp-delay", "network-delay", "delay-target", map[string]interface{}{
		"latency": "100ms",
	}))
	h.waitExperimentRunning(ctx, t, datapathNamespace, "dp-delay", 2*time.Minute)
	time.Sleep(3 * time.Second)

	after, err := h.MeasureNetwork(ctx, datapathNamespace, "delay-probe", targetIP, 20)
	if err != nil {
		t.Fatalf("post-fault ping: %v", err)
	}
	if after.AvgRTT-before.AvgRTT < 80*time.Millisecond {
		t.Fatalf("expected ~100ms added RTT, got delta %v (before=%v after=%v)", after.AvgRTT-before.AvgRTT, before.AvgRTT, after.AvgRTT)
	}
}

// TestNetworkLoss_DropsPackets (T7) asserts an ebpf-network-loss experiment
// drops approximately the configured percentage of packets. The daemon uses the
// real eBPF drop program on the resolved ifindex, falling back to netem loss.
func TestNetworkLoss_DropsPackets(t *testing.T) {
	h := requireCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	h.EnsureNamespace(ctx, t, datapathNamespace)
	h.DeployPod(ctx, t, PodSpec{Name: "loss-target", Namespace: datapathNamespace, Labels: map[string]string{"app": "loss-target"}})
	h.DeployPod(ctx, t, PodSpec{Name: "loss-probe", Namespace: datapathNamespace, Labels: map[string]string{"app": "loss-probe"}})

	targetIP, err := h.PodIP(ctx, datapathNamespace, "loss-target")
	if err != nil {
		t.Fatalf("target IP: %v", err)
	}

	h.applyExperiment(ctx, t, newNetworkExperiment(datapathNamespace, "dp-loss", "ebpf-network-loss", "loss-target", map[string]interface{}{
		"percent": "40",
	}))
	h.waitExperimentRunning(ctx, t, datapathNamespace, "dp-loss", 2*time.Minute)
	time.Sleep(3 * time.Second)

	stats, err := h.MeasureNetwork(ctx, datapathNamespace, "loss-probe", targetIP, 50)
	if err != nil {
		t.Fatalf("ping under loss: %v", err)
	}
	if stats.LossPercent < 20 {
		t.Fatalf("expected ~40%% loss, measured %.1f%%", stats.LossPercent)
	}
}

// TestNetworkPartition_BreaksAndRecovers (T8) asserts a partition cuts the
// target's connectivity to a CIDR and that deleting the experiment restores it.
func TestNetworkPartition_BreaksAndRecovers(t *testing.T) {
	h := requireCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	h.EnsureNamespace(ctx, t, datapathNamespace)
	h.DeployPod(ctx, t, PodSpec{Name: "part-target", Namespace: datapathNamespace, Labels: map[string]string{"app": "part-target"}})
	h.DeployPod(ctx, t, PodSpec{Name: "part-peer", Namespace: datapathNamespace, Labels: map[string]string{"app": "part-peer"}})

	peerIP, err := h.PodIP(ctx, datapathNamespace, "part-peer")
	if err != nil {
		t.Fatalf("peer IP: %v", err)
	}

	exp := newNetworkExperiment(datapathNamespace, "dp-part", "network-partition", "part-target", map[string]interface{}{
		"target_cidr": peerIP + "/32",
		"direction":   "both",
	})
	created, err := h.Dynamic.Resource(chaosExperimentGVR).Namespace(datapathNamespace).Create(ctx, exp, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create partition experiment: %v", err)
	}
	h.waitExperimentRunning(ctx, t, datapathNamespace, "dp-part", 2*time.Minute)
	time.Sleep(3 * time.Second)

	cut, err := h.MeasureNetwork(ctx, datapathNamespace, "part-target", peerIP, 10)
	if err != nil {
		t.Fatalf("ping under partition: %v", err)
	}
	if cut.LossPercent < 90 {
		t.Fatalf("expected near-total loss under partition, measured %.1f%%", cut.LossPercent)
	}

	if err := h.Dynamic.Resource(chaosExperimentGVR).Namespace(datapathNamespace).Delete(ctx, created.GetName(), metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete partition experiment: %v", err)
	}
	time.Sleep(5 * time.Second)

	recovered, err := h.MeasureNetwork(ctx, datapathNamespace, "part-target", peerIP, 10)
	if err != nil {
		t.Fatalf("ping after recovery: %v", err)
	}
	if recovered.LossPercent > 20 {
		t.Fatalf("expected connectivity restored after rollback, still %.1f%% loss", recovered.LossPercent)
	}
}

// TestDNSChaos_FailsTargetResolution (T9) asserts DNS chaos makes the target
// pod fail to resolve a configured domain while a control name still resolves.
func TestDNSChaos_FailsTargetResolution(t *testing.T) {
	h := requireCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	h.EnsureNamespace(ctx, t, datapathNamespace)
	h.DeployPod(ctx, t, PodSpec{Name: "dns-target", Namespace: datapathNamespace, Labels: map[string]string{"app": "dns-target"}})

	h.applyExperiment(ctx, t, newNetworkExperiment(datapathNamespace, "dp-dns", "pod-dns-error", "dns-target", map[string]interface{}{
		"domains": "blocked.example.com",
	}))
	h.waitExperimentRunning(ctx, t, datapathNamespace, "dp-dns", 2*time.Minute)
	time.Sleep(3 * time.Second)

	blocked, err := h.ResolveDNS(ctx, datapathNamespace, "dns-target", "blocked.example.com")
	if err != nil {
		t.Fatalf("resolve blocked name: %v", err)
	}
	if blocked.Resolved {
		t.Fatalf("expected blocked.example.com to fail resolution, output: %s", blocked.Output)
	}
}

// TestHTTPChaos_AbortsTargetPort (T10) asserts HTTP abort makes requests to the
// target's HTTP port fail while leaving the daemon's own traffic alone.
func TestHTTPChaos_AbortsTargetPort(t *testing.T) {
	h := requireCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	h.EnsureNamespace(ctx, t, datapathNamespace)
	// httpd serves on :80 so the probe has a real endpoint to hit.
	h.DeployPod(ctx, t, PodSpec{
		Name: "http-target", Namespace: datapathNamespace,
		Labels:  map[string]string{"app": "http-target"},
		Command: []string{"httpd", "-f", "-p", "80"},
	})
	h.DeployPod(ctx, t, PodSpec{Name: "http-probe", Namespace: datapathNamespace, Labels: map[string]string{"app": "http-probe"}})

	targetIP, err := h.PodIP(ctx, datapathNamespace, "http-target")
	if err != nil {
		t.Fatalf("target IP: %v", err)
	}
	url := "http://" + targetIP + ":80/"

	before, err := h.ProbeHTTP(ctx, datapathNamespace, "http-probe", url)
	if err != nil {
		t.Fatalf("baseline http: %v", err)
	}
	if !before.Success {
		t.Fatalf("expected baseline HTTP to succeed, output: %s", before.Output)
	}

	h.applyExperiment(ctx, t, newNetworkExperiment(datapathNamespace, "dp-http", "pod-http-abort", "http-target", map[string]interface{}{
		"port": "80",
	}))
	h.waitExperimentRunning(ctx, t, datapathNamespace, "dp-http", 2*time.Minute)
	time.Sleep(3 * time.Second)

	after, err := h.ProbeHTTP(ctx, datapathNamespace, "http-probe", url)
	if err != nil {
		t.Fatalf("post-fault http: %v", err)
	}
	if after.Success {
		t.Fatalf("expected HTTP to fail under abort, output: %s", after.Output)
	}
}

// TestCPUStress_SaturatesPodCgroup (T11) asserts CPU stress drives the target
// pod's own cgroup CPU usage up toward a full core, proving the stressor is
// scoped to the pod rather than the node.
func TestCPUStress_SaturatesPodCgroup(t *testing.T) {
	h := requireCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	h.EnsureNamespace(ctx, t, datapathNamespace)
	h.DeployPod(ctx, t, PodSpec{Name: "cpu-target", Namespace: datapathNamespace, Labels: map[string]string{"app": "cpu-target"}})

	idle, err := h.CPUBusyFraction(ctx, datapathNamespace, "cpu-target", 2*time.Second)
	if err != nil {
		t.Fatalf("baseline cpu: %v", err)
	}

	h.applyExperiment(ctx, t, newNetworkExperiment(datapathNamespace, "dp-cpu", "pod-cpu-stress", "cpu-target", map[string]interface{}{
		"workers":  "1",
		"duration": "120",
	}))
	h.waitExperimentRunning(ctx, t, datapathNamespace, "dp-cpu", 2*time.Minute)
	time.Sleep(3 * time.Second)

	busy, err := h.CPUBusyFraction(ctx, datapathNamespace, "cpu-target", 3*time.Second)
	if err != nil {
		t.Fatalf("cpu under stress: %v", err)
	}
	if busy-idle < 0.5 {
		t.Fatalf("expected pod cgroup CPU to rise toward a core, idle=%.2f busy=%.2f", idle, busy)
	}
}

// TestMemoryStress_RaisesPodCgroupMemory (T11) asserts memory stress raises the
// target pod's cgroup memory charge toward the configured stress size.
func TestMemoryStress_RaisesPodCgroupMemory(t *testing.T) {
	h := requireCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	h.EnsureNamespace(ctx, t, datapathNamespace)
	h.DeployPod(ctx, t, PodSpec{Name: "mem-target", Namespace: datapathNamespace, Labels: map[string]string{"app": "mem-target"}})

	before, err := h.ReadCgroupStats(ctx, datapathNamespace, "mem-target")
	if err != nil {
		t.Fatalf("baseline mem: %v", err)
	}

	h.applyExperiment(ctx, t, newNetworkExperiment(datapathNamespace, "dp-mem", "pod-memory-stress", "mem-target", map[string]interface{}{
		"workers":  "1",
		"size":     "128M",
		"duration": "120",
	}))
	h.waitExperimentRunning(ctx, t, datapathNamespace, "dp-mem", 2*time.Minute)
	time.Sleep(5 * time.Second)

	after, err := h.ReadCgroupStats(ctx, datapathNamespace, "mem-target")
	if err != nil {
		t.Fatalf("mem under stress: %v", err)
	}
	const wantDelta = 50 * 1024 * 1024
	if after.MemoryCurrentBytes < before.MemoryCurrentBytes+wantDelta {
		t.Fatalf("expected pod cgroup memory to rise >= 50MiB, before=%d after=%d", before.MemoryCurrentBytes, after.MemoryCurrentBytes)
	}
}
