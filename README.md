<p align="center">
  <img src="docs/assets/logo.png" alt="ChaosPlane" width="200" />
</p>

<h1 align="center">ChaosPlane</h1>

<p align="center">
  Open-source chaos engineering platform for Kubernetes.
  <br />
  Break things on purpose. Build confidence in production.
</p>

<p align="center">
  <a href="https://github.com/chaosplane-hq/chaosplane/actions/workflows/ci.yaml"><img src="https://github.com/chaosplane-hq/chaosplane/actions/workflows/ci.yaml/badge.svg" alt="CI" /></a>
  <a href="https://github.com/chaosplane-hq/chaosplane/releases"><img src="https://img.shields.io/github/v/release/chaosplane-hq/chaosplane" alt="Release" /></a>
  <a href="https://goreportcard.com/report/github.com/chaosplane-hq/chaosplane"><img src="https://goreportcard.com/badge/github.com/chaosplane-hq/chaosplane" alt="Go Report Card" /></a>
  <a href="https://pkg.go.dev/github.com/chaosplane-hq/chaosplane"><img src="https://img.shields.io/badge/go-1.24-blue.svg" alt="Go Version" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg" alt="License" /></a>
</p>

<p align="center">
  <a href="https://docs.chaosplane.dev">Documentation</a> · <a href="#quick-start">Quick Start</a> · <a href="https://github.com/chaosplane-hq/chaosplane/issues">Issues</a> · <a href="https://discord.gg/chaosplane">Discord</a>
</p>

---

## Features

- **20 chaos actions** — 8 pod, 6 network, 4 node, 2 stress fault injectors covering the full Kubernetes failure spectrum
- **DAG-based workflow engine** — Orchestrate multi-step experiments with dependencies, parallelism, delays, conditions, and suspend gates
- **Steady-state probes** — Validate system health before and after chaos with Prometheus, HTTP, and Kubernetes probes
- **BlastRadiusPolicy safety guardrails** — Enforce target limits, protect critical namespaces, restrict allowed actions, and define time windows
- **Time windows** — Schedule chaos during approved maintenance windows with cron-based allowed/blocked periods and timezone support
- **Abort conditions** — Automatically roll back experiments when monitored metrics breach thresholds
- **CLI (chaosctl)** — Create, inspect, abort, and manage experiments and workflows from the terminal
- **Web UI dashboard** — Real-time experiment monitoring, workflow visualization, and policy management built with Next.js and Carbon Design System

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                        │
│                                                                  │
│  ┌─────────────┐    ┌─────────────┐    ┌──────────────────────┐ │
│  │  Operator    │    │  Webhook    │    │  Daemon (DaemonSet)  │ │
│  │             │    │  (Validating│    │                      │ │
│  │  - Experiment│◄──►│   Admission)│    │  - Network faults    │ │
│  │    Controller│    └─────────────┘    │  - Stress injection  │ │
│  │  - Workflow  │                       │  - Node-level chaos  │ │
│  │    Controller│◄─────────────────────►│                      │ │
│  │  - 20 Exec- │         gRPC          └──────────────────────┘ │
│  │    utors     │                                                │
│  └──────┬───────┘                                                │
│         │                                                        │
│         │ CRDs: ChaosExperiment, ChaosWorkflow, BlastRadiusPolicy│
│         │                                                        │
├─────────┼────────────────────────────────────────────────────────┤
│         │              External Components                       │
│         │                                                        │
│  ┌──────┴───────┐    ┌─────────────┐    ┌─────────────────────┐ │
│  │  API Server  │◄──►│  Web UI     │    │  chaosctl (CLI)     │ │
│  │  (Gin REST)  │    │  (Next.js)  │    │                     │ │
│  └──────────────┘    └─────────────┘    └─────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
```

## Quick Start

### Install with Helm

```bash
helm install chaosplane oci://ghcr.io/chaosplane-hq/helm-charts/chaosplane \
  -n chaosplane-system --create-namespace
```

### Run your first experiment

Create a pod-kill experiment targeting pods with the label `app: my-app`:

```yaml
apiVersion: chaos.chaosplane.dev/v1alpha1
kind: ChaosExperiment
metadata:
  name: pod-kill-test
spec:
  target:
    kind: Pod
    namespace: default
    labelSelector:
      matchLabels:
        app: my-app
  action:
    type: pod-kill
    parameters: {}
  duration: 30s
```

Apply it:

```bash
kubectl apply -f experiment.yaml
```

Monitor the experiment:

```bash
# With chaosctl
chaosctl get experiments

# Or with kubectl
kubectl get chaosexperiments -w
```

## Chaos Actions

### Pod Actions (8)

| Action | Description | Target |
|--------|-------------|--------|
| `pod-kill` | Terminates target pods | Pod |
| `container-kill` | Kills a specific container inside a pod | Pod |
| `pod-cpu-stress` | Injects CPU pressure into pod cgroups | Pod |
| `pod-memory-stress` | Injects memory pressure into pod cgroups | Pod |
| `pod-io-stress` | Injects I/O pressure into pod cgroups | Pod |
| `pod-dns-error` | Injects DNS resolution failures | Pod |
| `pod-http-abort` | Aborts HTTP requests to/from pods | Pod |
| `pod-http-delay` | Adds latency to HTTP requests to/from pods | Pod |

### Network Actions (6)

| Action | Description | Target |
|--------|-------------|--------|
| `network-delay` | Adds network latency to pod traffic | Pod |
| `network-loss` | Drops a percentage of network packets | Pod |
| `network-corrupt` | Corrupts network packets | Pod |
| `network-duplicate` | Duplicates network packets | Pod |
| `network-partition` | Isolates pods from specific network targets | Pod |
| `network-bandwidth` | Limits network bandwidth | Pod |

### Node Actions (4)

| Action | Description | Target |
|--------|-------------|--------|
| `node-drain` | Cordons and drains a node | Node |
| `node-taint` | Applies taints to a node | Node |
| `node-restart` | Restarts a node | Node |
| `node-cpu-stress` | Injects CPU stress at the node level | Node |

### Stress Actions (2)

| Action | Description | Target |
|--------|-------------|--------|
| `stress-cpu` | Host-level CPU stress via stress-ng | Node |
| `stress-memory` | Host-level memory stress via stress-ng | Node |

## Custom Resource Definitions

ChaosPlane introduces three CRDs:

- **ChaosExperiment** — Defines a single chaos fault injection with target selection, action parameters, steady-state probes, and abort conditions. Follows an 8-phase state machine: `Pending` → `SteadyStateChecking` → `Running` → `Completing` → `Recovering` → `Completed` (or `Failed` / `Aborted`).

- **ChaosWorkflow** — Orchestrates multiple experiments as a DAG with five template types: `experiment`, `delay`, `condition`, `parallel`, and `suspend`. Supports error handling strategies (`abort`, `continue`, `retry`, `rollback`) and configurable parallelism.

- **BlastRadiusPolicy** — Enforces safety guardrails including target limits, protected namespaces/resources, allowed actions, maximum durations, and time windows with cron-based scheduling. Operates in `Enforce` or `Audit` mode.

## Configuration Examples

See [`config/samples/`](config/samples/) for ready-to-use examples:

- [`experiment_pod_kill.yaml`](config/samples/experiment_pod_kill.yaml) — Simple pod-kill experiment
- [`experiment_network_delay.yaml`](config/samples/experiment_network_delay.yaml) — Network delay injection
- [`workflow_basic.yaml`](config/samples/workflow_basic.yaml) — Multi-step workflow with DAG dependencies
- [`policy_production.yaml`](config/samples/policy_production.yaml) — Production safety policy with time windows

## Documentation

Full documentation is available at [docs.chaosplane.dev](https://docs.chaosplane.dev), including:

- [Getting Started Guide](https://docs.chaosplane.dev/docs/getting-started)
- [Action Reference](https://docs.chaosplane.dev/docs/actions)
- [Workflow Guide](https://docs.chaosplane.dev/docs/workflows)
- [Policy Configuration](https://docs.chaosplane.dev/docs/policies)
- [CLI Reference](https://docs.chaosplane.dev/docs/cli)
- [API Reference](https://docs.chaosplane.dev/docs/api)

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for local development setup with kind and Tilt.

```bash
make setup    # Install tools + create kind cluster
make dev      # Start Tilt dev server
make test     # Run unit tests
```

## Community

- [Discord](https://discord.gg/chaosplane) — Chat with the community and maintainers
- [GitHub Discussions](https://github.com/chaosplane-hq/chaosplane/discussions) — Ask questions, share ideas
- [Twitter / X](https://x.com/chaosplane_hq) — Follow for updates

## Contributing

We welcome contributions of all kinds. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to get started.

## Security

To report a security vulnerability, please see [SECURITY.md](SECURITY.md). Do not open a public issue.

## License

ChaosPlane is licensed under the [Apache License 2.0](LICENSE).

Copyright 2024-2025 ChaosPlane Contributors.
