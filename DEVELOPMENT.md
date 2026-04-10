# Development Guide

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.24+ | https://go.dev/dl/ |
| Docker | 24+ | https://docs.docker.com/get-docker/ |
| kind | 0.25+ | `go install sigs.k8s.io/kind@v0.25.0` |
| kubectl | 1.31+ | https://kubernetes.io/docs/tasks/tools/ |
| Tilt | 0.33+ | https://docs.tilt.dev/install.html |
| buf | 1.47+ | https://buf.build/docs/installation |

## Quick Start

```bash
# 1. Install dev tools and create kind cluster
make setup

# 2. Start platform services (postgres, redis, etc.)
docker compose -f ../chaosplane-platform/deploy/docker-compose.yaml up -d

# 3. Start Tilt for hot-reload development
make dev
```

## Make Targets

Run `make help` to see all available targets.

| Target | Description |
|--------|-------------|
| `setup` | Install tools + create kind cluster |
| `dev` | Start Tilt dev server |
| `build` | Build all binaries to `bin/` |
| `test` | Unit tests with coverage |
| `test-integration` | Integration tests |
| `e2e` | E2E tests against kind cluster |
| `lint` | Run golangci-lint |
| `fmt` | Format code with gofumpt |
| `generate` | Generate deepcopy methods |
| `manifests` | Generate CRD, RBAC, webhook YAML |
| `docker-build` | Build all container images |
| `docker-push` | Push images to ghcr.io/chaosplane-hq |
| `clean` | Remove build artifacts |

## Local Development Workflow

1. `make setup` — one-time setup of tools and kind cluster
2. `make dev` — starts Tilt, which watches for file changes and:
   - Rebuilds Go binaries on source changes
   - Syncs binaries into running containers
   - Re-applies CRD manifests when API types change
3. Edit code → Tilt auto-rebuilds → changes are live in the cluster

Build a single component:

```bash
make build-operator
make build-agent
```

## Code Generation

After modifying API types in `api/`:

```bash
make generate   # deepcopy methods
make manifests  # CRD + RBAC + webhook YAML
```

Generated files (`zz_generated.deepcopy.go`, CRD YAMLs) are managed exclusively by these targets. Do not edit them by hand.

## Testing

```bash
make test                # unit tests with coverage report (cover.out)
make test-integration    # integration tests (requires running services)
make e2e                 # end-to-end tests (requires kind cluster)
```

## Container Images

```bash
make docker-build                # build all images
make docker-build-operator       # build a single image
TAG=v0.1.0 make docker-build    # custom tag (default: dev)
```
