# ChaosPlane Limitations and Honest Caveats

ChaosPlane aims to inject real faults, not to pretend. This document explains
the nuances behind the mechanisms: where eBPF is and is not used, what must be
deployed before a fault does anything, and how the implementation compares to
the original project plan. Treat it as a companion to
[CAPABILITIES.md](CAPABILITIES.md).

Honesty here is a feature. A chaos tool that silently reports success while
doing nothing is worse than one that tells you exactly what ran.

## The eBPF reality

The original differentiator was that network chaos would run on eBPF and so
need only `CAP_BPF` + `CAP_NET_ADMIN`, never a privileged container. That
thesis holds for the packet-drop path. The details matter, so here they are in
full.

### What IS eBPF

`ebpf-network-loss` is a real eBPF program. The daemon builds a TC classifier
from hand-written `cilium/ebpf` `asm.Instructions`, loads it as a `SchedCLS`
program, and attaches it to the **TC ingress hook of the resolved host-side
veth** using `link.AttachTCX`:

- Program instructions: `internal/daemon/ebpf/programs.go`. It calls
  `bpf_get_prandom_u32`, takes it `% 100`, and returns `TC_ACT_SHOT` (drop)
  when the result is below the configured percent, otherwise `TC_ACT_OK`. That
  is a genuine probabilistic packet drop in the kernel datapath.
- Load and attach: `internal/daemon/ebpf/manager.go:35-66`. It removes the
  memlock rlimit, creates the program, and attaches via TCX ingress on the
  host-side veth ifindex.
- Dispatch: `internal/daemon/server.go:98-119`, gated on `mode=ebpf` and
  `action=loss`.

Attaching on the host-side veth's ingress means the pod's **egress** traffic
(packets leaving the pod, arriving at the host peer) is what gets dropped
(`internal/daemon/ebpf/manager.go:51-57`).

This path needs only `CAP_BPF` + `CAP_NET_ADMIN`. It does not enter the pod's
network namespace, does not run a privileged container, and does not shell out
to `tc`. For the drop fault, the no-privileged thesis is real.

### What is NOT eBPF

Being precise about the boundaries:

- **Network delay is `tc netem`, not eBPF.** A TC classifier cannot sleep or
  queue a packet for later, so there is no way to express "hold this packet for
  100ms" in the BPF program. Delay is therefore implemented with the kernel's
  `netem` qdisc. `ebpf-network-delay` carries an `ebpf-` name for historical
  reasons but dispatches to `tc netem delay`
  (`internal/executor/network/ebpf_delay.go:30`,
  `internal/daemon/server.go:94-97,120-129`). The comment at
  `internal/daemon/server.go:94-97` states this directly.
- **`ebpf-dns-chaos` is `iptables`, not eBPF.** It uses the same
  iptables-string-match DROP path as `pod-dns-error`
  (`internal/executor/network/ebpf_dns.go:31`, `internal/daemon/syscmd.go:307-343`).
- **The eBPF program is hand-written `asm.Instructions`, not bpf2go / CO-RE
  C.** There is no compiled C object, no `clang` step, no CO-RE relocation. The
  program is assembled in Go at runtime (`internal/daemon/ebpf/programs.go`).
  This avoids a `clang`/LLVM toolchain at build time, at the cost of expressing
  only simple logic.
- **It is TC, not XDP.** The plan said "TC Hook + XDP." The implementation uses
  the TC (`SchedCLS` / TCX) hook only. There is no XDP program
  (`internal/daemon/ebpf/manager.go:39-57`).

### The netem fallback

eBPF attach can fail on kernels without TCX support or in environments without
`CAP_BPF`. Rather than fail the experiment, the daemon falls back to `tc netem
loss` and records which datapath actually ran in the execution parameters
(`datapath=ebpf` vs `datapath=netem-fallback`,
`internal/daemon/server.go:105-119`). Cleanup reads that field so it tears down
the right thing: `ebpfMgr.Unload` for eBPF, `tc qdisc del` for netem
(`internal/daemon/server.go:448-455`).

The honest consequence: when the fallback fires, the no-privileged property may
not hold, because `tc` needs `CAP_NET_ADMIN` and operates differently. The
system tells you which path took effect instead of hiding it.

## Operational requirements

A registered executor is necessary but not sufficient. Several faults only do
something real once the deployment is wired correctly.

### Daemon CRI socket + netns resolver

Pod-scoped network, DNS, HTTP, and stress faults must resolve the pod's
host-side veth and cgroup before they can act. The daemon does this server-side
through a CRI runtime socket and netlink, with no privileged container and no
`setns` into the pod (`internal/daemon/netns/resolver_linux.go:30-59`).

The resolver is only active when the daemon is started with `--cri-endpoint`
pointing at the containerd/CRI socket (`cmd/daemon/main.go:24,46-54`). Until
then:

- A pod-targeted network fault returns `Success: false` with `"no CRI resolver
  configured on this daemon"` (`internal/daemon/server.go:167-173`).
- DNS/HTTP/stress faults likewise return failure rather than acting on the
  daemon's own interface (`internal/daemon/server.go:214-216`).

This is deliberate. Faulting the wrong interface (the daemon's own `eth0`)
would be both useless and dangerous, so the daemon refuses instead.

### DaemonSet capabilities

The kernel-level faults need the daemon to run with the right Linux
capabilities on each node:

- eBPF drop: `CAP_BPF` (plus memlock, removed at startup in
  `internal/daemon/ebpf/manager.go:26-28`).
- `tc` / `iptables` / netns work: `CAP_NET_ADMIN`.
- cgroup-scoped stress: write access to the host cgroup v2 hierarchy at
  `/sys/fs/cgroup` (`internal/daemon/syscmd.go:281-295`).

These are narrower than a privileged container, which is the point, but they
are still privileges your cluster policy must allow.

### Cloud credentials and reach

- **AWS faults** need `awsRegion` plus credentials, via static keys in
  parameters or the operator's ambient AWS credential chain
  (`internal/executor/aws/client.go:20-40`). No credentials means the SDK call
  fails and the experiment reports the error.
- **GCP faults** authenticate with a caller-supplied bearer token
  (`gcpBearerToken`), not application-default credentials
  (`internal/executor/gcp/client.go:19-25,41`).
- **Azure faults** need `subscriptionId` + `resourceGroup` and SDK credentials
  (`internal/executor/azure/azure.go:13-29`).
- **VM faults** need SSH reachability and a key you control; they shell out to
  `ssh` (`internal/executor/vm/vm.go:40-60`). They do not work against managed
  control planes you cannot SSH into.

### Irreversible and partial faults

- `aws-ec2-terminate` cannot be rolled back; its `Rollback` is intentionally a
  no-op (`internal/executor/aws/aws.go:159-161`). Use with care.
- `aws-az-failure` currently describes subnets in the AZ and logs intent
  without applying a deny rule (`internal/executor/aws/aws.go:266-290`).
- `node-restart` is registered and sends a daemon RPC, but the daemon does not
  implement the `restart` action on this branch, so it returns `Success: false`
  (`internal/executor/node/restart.go:63-65` vs
  `internal/daemon/server.go:391-406`).

## Reconciling with the original plan

The project plan (`beginning_plan.md`) set out an ambitious surface. Here is a
fair accounting of where the implementation matches it and where it diverged,
with the reasoning. This is a record of engineering decisions, not an apology.

### Network chaos via eBPF "TC Hook + XDP", no privileged container

> Plan (§2.2): network chaos is written with cilium/ebpf as eBPF TC Hook and
> XDP programs; unlike tc-netem (which needs a privileged container), eBPF runs
> with only `CAP_BPF` + `CAP_NET_ADMIN`.

**What we built:** A real eBPF TC (SchedCLS/TCX) packet-drop program for
`network-loss`, attached to the host-side veth, needing only `CAP_BPF` +
`CAP_NET_ADMIN`. No XDP. Delay, partition, DNS, and HTTP run on `tc netem` and
`iptables` on the host-side veth, also without a privileged container.

**Why:**

- *Delay can't be eBPF.* A TC classifier cannot sleep, so packet delay is only
  expressible with the kernel's `netem` qdisc. We removed an earlier eBPF
  "delay" that was effectively a no-op rather than ship something that looked
  like eBPF but did nothing.
- *TC, not XDP.* The drop fault targets pod traffic on the host-side veth,
  where the TC hook is the natural attach point. XDP would add complexity
  without changing the fault's behavior here.
- *Hand-written asm, not bpf2go/CO-RE C.* Assembling the program in Go avoids
  requiring a `clang`/LLVM toolchain in the build, which keeps the build simple
  for a probabilistic drop that needs only a handful of instructions.

The core promise (chaos without a privileged container) holds for the eBPF drop
path and for the netem/iptables paths, which use scoped capabilities rather
than `privileged: true`.

### Fault layers (Pod, Network, K8s cluster, Cloud)

**What we built:** Pod, network, node, host-stress, and multi-cloud (AWS, GCP,
Azure, VM-over-SSH) faults are present and registered
(`cmd/operator/main.go:60-107`).

**Divergences worth naming:**

- `TimeChaos` and `EtcdLatency` from the plan's K8s-layer list are **not
  present on this branch**. There is no `time-chaos` or etcd executor in
  `internal/executor` or `cmd/operator/main.go`. They are not documented as
  working because they do not exist here.
- Several cloud actions in the plan (Lambda, S3, ElastiCache, EKS-specific) are
  not individually implemented; the AWS set is EC2, RDS, ECS, and an AZ-failure
  scaffold.

### The web UI, topology visualization, metric comparison

These are product-layer features described in the plan. This repository is the
chaos engine (operator, daemon, executors, CRDs, CLI). UI and metric-comparison
claims are out of scope for this engine's capability docs and are not asserted
here.

## Summary

ChaosPlane injects real faults through real kernel and API mechanisms. The
honest edges:

- One real eBPF program (`network-loss` drop), TC not XDP, hand-written asm not
  CO-RE C, with a netem fallback that is reported when it fires.
- All other network faults are `tc netem` / `iptables`, still without a
  privileged container.
- Pod-scoped faults need the daemon's CRI socket wired, or they return
  `Success: false`.
- Cloud faults need real credentials; some are irreversible or partial.
- `TimeChaos` and `EtcdLatency` are not in this build.

If a fault is not real yet, it says so at runtime rather than reporting a
success it did not earn (`internal/daemon/honesty_test.go`).
