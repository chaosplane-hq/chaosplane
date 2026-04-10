# ChaosPlane

Open-source chaos engineering platform for Kubernetes.

## Components

- **Operator** — Kubernetes controller managing chaos experiments
- **Agent** — Cluster agent connecting to the ChaosPlane control plane
- **Daemon** — DaemonSet for node-level chaos (network, stress)
- **chaosctl** — CLI for managing experiments

## Quick Start

```bash
make setup    # Install tools + create kind cluster
make dev      # Start Tilt dev server
```

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for detailed setup instructions.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
