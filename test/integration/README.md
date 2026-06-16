# Integration Test Harness

Kind-backed integration tests that assert the **real effects** of chaos faults:
network degradation, cgroup resource pressure, and pod lifecycle. Fault tests
(T6–T11) and the final verification (T20) compose the exported helpers here
instead of re-implementing measurement logic.

## Running

These tests require a live cluster and are gated behind both the `integration`
build tag and the `INTEGRATION=1` environment variable, so `go test ./...` in CI
(without kind) stays green while the harness still runs when invoked properly.

```sh
# 1. Bring up a kind cluster (installs tools + cluster)
make setup
# or, if tools are already present:
bash hack/setup-kind.sh

# 2. Deploy the operator + daemon so experiments actually execute
#    (helm install / tilt up — see the repo root README)

# 3. Run the harness
make test-integration
# equivalent to:
INTEGRATION=1 go test -race -tags=integration -count=1 -timeout=30m ./test/integration/...
```

`KUBECONFIG` is honored; it defaults to `~/.kube/config`. Without `INTEGRATION=1`
the tests skip cleanly.

### Self-verification

`TestHarnessSelfVerify_PodKill` is the harness's end-to-end smoke test. It
deploys a target pod, runs a `pod-kill` ChaosExperiment, and asserts the target
pod is actually deleted. Run it alone:

```sh
INTEGRATION=1 go test -tags=integration -run TestHarnessSelfVerify_PodKill -v ./test/integration/...
```

## Assertion API

All helpers hang off `*Harness` (build one in a test via `requireCluster(t)`).

### Cluster & workloads

| Helper | Purpose |
| --- | --- |
| `requireCluster(t)` | Skip unless `INTEGRATION=1`; returns a connected `*Harness`. |
| `Harness.EnsureNamespace(ctx, t, ns)` | Create a namespace (idempotent) with auto-cleanup. |
| `Harness.DeployPod(ctx, t, PodSpec)` | Deploy a target or probe pod, wait until Ready, auto-cleanup. Defaults to a busybox image with `ping`/`nslookup`/`wget`. |
| `Harness.WaitForPodReady(ctx, ns, name, timeout)` | Block until a pod is Ready. |
| `Harness.PodIP(ctx, ns, name)` | Return a pod's cluster IP for use as a network target. |
| `Harness.Exec(ctx, ns, pod, cmd...)` | Run a command in a pod; returns `ExecResult{Stdout, Stderr, ExitCode}`. Non-zero exits are reported, not errored. |

### Network effects

| Helper | Returns | Assert on |
| --- | --- | --- |
| `Harness.MeasureNetwork(ctx, ns, probePod, target, count)` | `NetworkStats{LossPercent, AvgRTT, MinRTT, MaxRTT, ...}` | RTT rose by ~X ms; loss ≈ Y%. |
| `Harness.ResolveDNS(ctx, ns, probePod, name)` | `DNSResult{Resolved, Output}` | DNS fails for a target name but a control name still resolves. |
| `Harness.ProbeHTTP(ctx, ns, probePod, url)` | `HTTPResult{Success, Latency, ...}` | HTTP success flips, or latency rises. |

Typical before/after pattern (use the same `count` both times):

```go
before, _ := h.MeasureNetwork(ctx, ns, "probe", targetIP, 20)
// ... apply a network-delay fault ...
after, _ := h.MeasureNetwork(ctx, ns, "probe", targetIP, 20)
if after.AvgRTT-before.AvgRTT < 80*time.Millisecond {
    t.Fatalf("expected ~100ms added latency, got delta %v", after.AvgRTT-before.AvgRTT)
}
```

### Cgroup pressure

| Helper | Returns | Assert on |
| --- | --- | --- |
| `Harness.ReadCgroupStats(ctx, ns, pod)` | `CgroupStats{CPUUsageUsec, MemoryCurrentBytes}` | Memory charge rises toward the stress size. |
| `Harness.CPUBusyFraction(ctx, ns, pod, window)` | `float64` (fraction of one core) | CPU-stress saturates ≈1.0 core. |

Stats are read from the pod's own cgroup v2 hierarchy (`/sys/fs/cgroup`), which
is what kind nodes expose, so they reflect the pressure the fault actually
imposed rather than the kubelet's aggregate view.
