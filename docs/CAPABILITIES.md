# ChaosPlane Fault Capability Reference

This document describes exactly what each ChaosPlane fault does, the real
mechanism that backs it, and how mature that path is. Every row is grounded in
source on this branch, with file citations. The goal is honesty: if a fault
needs wiring before it does anything, that is stated plainly rather than
implied to "just work."

For the nuances behind these mechanisms (the eBPF reality, what must be
deployed, plan-vs-implementation), see [LIMITATIONS.md](LIMITATIONS.md).

## How faults reach the host

An experiment names a target (pods, nodes, or a cloud resource). The operator
resolves it and dispatches through one of two paths:

- **Daemon path** — for faults that act on a node's kernel (network, stress,
  DNS, HTTP, node-level). The operator calls the per-node chaos-daemon over
  gRPC, and the daemon runs the real kernel-level operation. Daemon RPCs live
  in `internal/daemon/server.go`.
- **API path** — for faults that act through an external control plane (the
  Kubernetes API for node cordon/drain, or a cloud provider SDK/REST API). The
  operator calls the API directly; no daemon involved. Executors live under
  `internal/executor/{node,aws,gcp,azure,vm}`.

The daemon only acts on a specific pod once its CRI socket and netns resolver
are wired (see [Operational requirements](LIMITATIONS.md#operational-requirements)).
Without that, a pod-targeted fault honestly returns `Success: false` instead of
faulting the wrong interface (`internal/daemon/server.go:167-173`).

## Maturity levels

| Level | Meaning |
|-------|---------|
| **production-real** | Performs a real kernel/API operation today, given a correctly deployed daemon or valid credentials. |
| **requires-deployment-wiring** | The mechanism is real, but only acts on a pod when the daemon's CRI socket + netns resolver are configured. Without wiring, returns `Success: false`. |
| **cloud-only** | Calls a cloud provider API. Needs valid provider credentials and reachability; nothing runs in-cluster. |
| **requires-self-hosted** | Needs SSH or host access you control (not available on managed control planes). |
| **partial** | Dispatched by an executor but not fully handled on the daemon side on this branch. See notes. |

## Pod faults

| Action | Real mechanism | Maturity | Source |
|--------|----------------|----------|--------|
| `pod-kill` | Kubernetes API pod deletion | production-real | `internal/executor/pod/kill.go`, registered `cmd/operator/main.go:60` |
| `container-kill` | Daemon RPC to kill a container in the pod | requires-deployment-wiring | `internal/executor/pod/container_kill.go`, `cmd/operator/main.go:61` |
| `pod-cpu-stress` | `stress-ng --cpu` launched inside the pod's cgroup v2 | requires-deployment-wiring | `internal/executor/pod/cpu_stress.go`, `internal/daemon/syscmd.go:235-273` |
| `pod-memory-stress` | `stress-ng --vm` inside the pod's cgroup v2 | requires-deployment-wiring | `internal/executor/pod/memory_stress.go`, `internal/daemon/syscmd.go:247-256` |
| `pod-io-stress` | `stress-ng --hdd` inside the pod's cgroup v2 | requires-deployment-wiring | `internal/executor/pod/io_stress.go`, `internal/daemon/syscmd.go:257-262` |
| `pod-dns-error` | `iptables` DROP of DNS (UDP+TCP/53) matched per-domain on the pod's host-side veth | requires-deployment-wiring | `internal/executor/pod/dns_error.go`, `internal/daemon/syscmd.go:307-343` |
| `pod-http-abort` | `iptables` REJECT of TCP to the HTTP port in the FORWARD chain scoped to the pod's host-side veth | requires-deployment-wiring | `internal/executor/pod/http_abort.go`, `internal/daemon/syscmd.go:115-126` |
| `pod-http-delay` | `tc netem delay` on the pod's host-side veth | requires-deployment-wiring | `internal/executor/pod/http_delay.go`, `internal/daemon/syscmd.go:128-138` |

Stress faults are cgroup-scoped: the daemon migrates the launched process into
the pod's cgroup v2 by writing its PID to `cgroup.procs`, so the stressor runs
under the pod's CPU/memory accounting rather than the whole node
(`internal/daemon/syscmd.go:275-295`). A pod that resolves but has an empty
cgroup path is refused rather than stressing the node
(`internal/daemon/server.go:251-257`).

## Network faults

| Action | Real mechanism | Maturity | Source |
|--------|----------------|----------|--------|
| `network-delay` | `tc netem delay` on the pod's host-side veth | requires-deployment-wiring | `internal/daemon/syscmd.go:53-65`, dispatch `internal/daemon/server.go:120-129` |
| `network-loss` | `tc netem loss` on the pod's host-side veth | requires-deployment-wiring | `internal/daemon/syscmd.go:66-74` |
| `network-corrupt` | `tc netem corrupt` | requires-deployment-wiring | `internal/daemon/syscmd.go:75-83` |
| `network-duplicate` | `tc netem duplicate` | requires-deployment-wiring | `internal/daemon/syscmd.go:84-92` |
| `network-bandwidth` | `tc tbf rate` token-bucket shaping | requires-deployment-wiring | `internal/daemon/syscmd.go:93-99` |
| `network-partition` | `iptables` DROP in FORWARD keyed on the pod's host-side veth and a target CIDR, per direction | requires-deployment-wiring | `internal/daemon/server.go:67-92`, `internal/daemon/syscmd.go:185-220` |
| `ebpf-network-loss` | **eBPF** packet drop: a `SchedCLS` program built from `cilium/ebpf` `asm.Instructions`, attached to TC ingress of the host-side veth. Falls back to `tc netem loss` if eBPF attach fails. | requires-deployment-wiring | `internal/daemon/ebpf/programs.go`, `internal/daemon/ebpf/manager.go`, `internal/daemon/server.go:98-119` |
| `ebpf-network-delay` | **Not eBPF.** Runs `tc netem delay` (a TC classifier cannot sleep). The `ebpf-` prefix is historical. | requires-deployment-wiring | `internal/executor/network/ebpf_delay.go:30`, `internal/daemon/server.go:94-97,120-129` |
| `ebpf-dns-chaos` | `iptables` DNS DROP (same path as `pod-dns-error`); not an eBPF datapath. | requires-deployment-wiring | `internal/executor/network/ebpf_dns.go:31`, `internal/daemon/server.go` DNS dispatch |

Only `ebpf-network-loss` runs a real eBPF program, and only when its request
carries `mode=ebpf` with `action=loss` (`internal/daemon/server.go:98`). Every
other network action, including `ebpf-network-delay`, runs through `tc netem`
or `iptables`. The full reasoning is in
[The eBPF reality](LIMITATIONS.md#the-ebpf-reality).

## Node faults

| Action | Real mechanism | Maturity | Source |
|--------|----------------|----------|--------|
| `node-drain` | Kubernetes API: cordon (`Unschedulable=true`) + Eviction API per pod | production-real | `internal/executor/node/drain.go:61-132` |
| `node-taint` | Kubernetes API: applies taints to the node | production-real | `internal/executor/node/taint.go`, `cmd/operator/main.go:77` |
| `node-cpu-stress` | Daemon RPC → `stress-ng --cpu` at node scope (no cgroup path) | requires-deployment-wiring | `internal/executor/node/cpu_stress.go:56-64`, `internal/daemon/server.go:392-393` |
| `node-restart` | Dispatched with daemon action `"restart"`, which the daemon does **not** handle on this branch → `Success: false`. | partial | `internal/executor/node/restart.go:63-65` vs `internal/daemon/server.go:391-406` |

`node-restart` is registered (`cmd/operator/main.go:78`) and sends a real RPC,
but `ExecNodeChaos` only implements `cpu-stress` and `partition`; an unknown
action returns `Success: false` with `"unsupported node chaos action: restart"`
(`internal/daemon/server.go:400-405`). Documented as `partial` rather than
implied complete.

## Host-level stress faults

| Action | Real mechanism | Maturity | Source |
|--------|----------------|----------|--------|
| `stress-cpu` | Daemon RPC → `stress-ng --cpu` at host scope | requires-deployment-wiring | `internal/executor/stress/cpu.go`, `internal/daemon/syscmd.go:235-273` |
| `stress-memory` | Daemon RPC → `stress-ng --vm` at host scope | requires-deployment-wiring | `internal/executor/stress/memory.go`, `cmd/operator/main.go:82` |

## Cloud faults (AWS)

All AWS faults use the AWS SDK v2 and need valid credentials/IAM (static keys
in params, or the daemon/operator's ambient credential chain) plus an
`awsRegion` parameter (`internal/executor/aws/client.go:20-40`,
`internal/executor/aws/aws.go:39-53`).

| Action | Real mechanism | Maturity | Source |
|--------|----------------|----------|--------|
| `aws-ec2-stop` | `ec2:StopInstances`; rollback starts them again | cloud-only | `internal/executor/aws/aws.go:77-121` |
| `aws-ec2-terminate` | `ec2:TerminateInstances` (irreversible; no rollback) | cloud-only | `internal/executor/aws/aws.go:138-161` |
| `aws-rds-failover` | `rds:FailoverDBCluster` | cloud-only | `internal/executor/aws/aws.go:178-201` |
| `aws-ecs-stop-task` | `ecs:StopTask` | cloud-only | `internal/executor/aws/aws.go:222-245` |
| `aws-az-failure` | `ec2:DescribeSubnets` for the AZ, then logs intent. **Does not yet sever the AZ**; it is a discovery/scaffold step. | partial / cloud-only | `internal/executor/aws/aws.go:266-290` |

`aws-ec2-terminate` is irreversible (`Rollback` is a no-op,
`internal/executor/aws/aws.go:159-161`). `aws-az-failure` describes subnets and
logs `"simulating AZ failure"` without applying a deny rule, so it is `partial`.

## Cloud faults (GCP, Azure, VM)

| Action | Real mechanism | Maturity | Source |
|--------|----------------|----------|--------|
| `gcp-gke-scale` | GKE REST `nodePools/setSize` via bearer token | cloud-only | `internal/executor/gcp/client.go:59-65` |
| `gcp-cloudsql-failover` | Cloud SQL REST `instances/failover` | cloud-only | `internal/executor/gcp/client.go:67-77` |
| `gcp-cloudrun-stop` | Cloud Run REST traffic update | cloud-only | `internal/executor/gcp/client.go:79-88` |
| `azure-vm-stop` | Azure SDK VM stop | cloud-only | `internal/executor/azure/azure.go` |
| `azure-aks-scale` | Azure SDK AKS node-pool scale | cloud-only | `internal/executor/azure/azure.go` |
| `azure-cosmosdb-failover` | Azure SDK Cosmos DB failover | cloud-only | `internal/executor/azure/azure.go` |
| `vm-cpu-stress` | SSH into a host and run a stressor | requires-self-hosted | `internal/executor/vm/vm.go:40-60` |
| `vm-memory-stress` | SSH stressor | requires-self-hosted | `internal/executor/vm/vm.go` |
| `vm-disk-stress` | SSH stressor | requires-self-hosted | `internal/executor/vm/vm.go` |
| `vm-network-delay` | SSH `tc netem` on a remote host | requires-self-hosted | `internal/executor/vm/vm.go` |
| `vm-process-kill` | SSH `kill` | requires-self-hosted | `internal/executor/vm/vm.go` |
| `vm-process-suspend` | SSH `kill -STOP` | requires-self-hosted | `internal/executor/vm/vm.go` |

GCP faults authenticate with a caller-supplied bearer token
(`gcpBearerToken`), not application-default credentials
(`internal/executor/gcp/client.go:19-25,41`). VM faults need SSH reachability
and a key you control (`internal/executor/vm/vm.go:40-60`).

## What honesty looks like at runtime

Every daemon RPC returns `Success: false` with an actionable message when the
underlying command fails, rather than reporting a fault that never applied.
This is enforced by `internal/daemon/honesty_test.go`, which asserts false on
command failure and true only on real success across network, stress, DNS,
HTTP, and node paths.
