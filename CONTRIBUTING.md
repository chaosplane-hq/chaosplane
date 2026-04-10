# Contributing to ChaosPlane

Thank you for your interest in contributing to ChaosPlane! This guide will help you get started.

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| [Go](https://go.dev/dl/) | 1.24+ | Core language |
| [Docker](https://docs.docker.com/get-docker/) | 24+ | Container builds |
| [kind](https://kind.sigs.k8s.io/) | 0.25+ | Local Kubernetes cluster |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | 1.31+ | Cluster interaction |
| [Helm](https://helm.sh/docs/intro/install/) | 3.16+ | Chart testing |
| [Tilt](https://docs.tilt.dev/install.html) | 0.33+ | Hot-reload dev loop |
| [pnpm](https://pnpm.io/installation) | 9+ | Web UI dependencies |

## Getting Started

### 1. Fork and clone

```bash
git clone https://github.com/<your-username>/chaosplane.git
cd chaosplane
git remote add upstream https://github.com/chaosplane-hq/chaosplane.git
```

### 2. Set up the development environment

```bash
# Install dev tools (controller-gen, golangci-lint, gofumpt, buf) and create a kind cluster
make setup

# Start Tilt for hot-reload development
make dev
```

See [DEVELOPMENT.md](DEVELOPMENT.md) for the full development guide.

### 3. Create a feature branch

```bash
git checkout -b feat/my-feature
```

## Code Style

- Go code follows [Effective Go](https://go.dev/doc/effective_go) conventions
- Format with `gofumpt`: `make fmt`
- Lint with `golangci-lint`: `make lint` (configuration in [`.golangci.yml`](.golangci.yml))
- All exported types and functions must have doc comments
- Keep functions focused — prefer small, testable units

## Testing

Run the full test suite before submitting a PR:

```bash
# Unit tests with coverage
make test

# Integration tests (requires running services)
make test-integration

# End-to-end tests (requires kind cluster)
make e2e
```

Write tests for all new functionality. Place unit tests alongside the code they test (`*_test.go`). Place integration and e2e tests under `test/`.

## Code Generation

If you modify API types in `api/v1alpha1/`:

```bash
make generate    # Regenerate deepcopy methods
make manifests   # Regenerate CRD, RBAC, and webhook YAML
```

Always commit generated files alongside your changes. CI will verify they are up to date.

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add network-reorder chaos action
fix: prevent experiment from stalling in Recovering phase
docs: add workflow DAG examples to getting started guide
refactor: extract probe evaluation into dedicated package
test: add e2e tests for node-drain executor
chore: bump controller-gen to v0.16.5
```

Scope is optional but encouraged for clarity:

```
feat(executor): add pod-http-delay action
fix(controller): handle nil rollback spec gracefully
```

## Pull Request Process

1. Ensure your branch is up to date with `main`:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. Run all checks locally:
   ```bash
   make fmt
   make lint
   make test
   make generate manifests && git diff --exit-code
   ```

3. Push your branch and open a PR against `main`.

4. Fill in the PR template with a description of your changes, motivation, and any testing performed.

5. PRs require:
   - At least 1 approving review from a maintainer
   - All CI checks passing (lint, test, generate-check)
   - No merge conflicts

6. Keep PRs focused — one feature or fix per PR. Large changes should be broken into a series of smaller PRs.

## Adding a Custom Chaos Action

ChaosPlane's executor system is designed for extensibility. To add a new chaos action:

### 1. Implement the `Executor` interface

Create a new file in the appropriate package under `internal/executor/`:

```go
package pod // or network, node, stress

import (
    "context"
    "log/slog"

    v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
    "github.com/chaosplane-hq/chaosplane/internal/executor"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type MyActionExecutor struct {
    logger *slog.Logger
    client client.Client
}

func NewMyActionExecutor(logger *slog.Logger, client client.Client) executor.Executor {
    return &MyActionExecutor{logger: logger, client: client}
}

func (e *MyActionExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
    // Inject the fault
    return nil
}

func (e *MyActionExecutor) Recover(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
    // Clean up / rollback the fault
    return nil
}
```

### 2. Register the executor in `cmd/operator/main.go`

```go
registry.MustRegister("my-action", pod.NewMyActionExecutor(logger, k8sClient))
```

### 3. Add tests

- Unit test in `internal/executor/<category>/my_action_test.go`
- E2E test in `test/e2e/` if the action requires a real cluster

### 4. Update documentation

- Add the action to the table in `README.md`
- Add a sample YAML in `config/samples/`
- Document parameters on the docs site

## Reporting Issues

Use [GitHub Issues](https://github.com/chaosplane-hq/chaosplane/issues) to report bugs or request features. Include:

- Steps to reproduce the issue
- Expected vs. actual behavior
- Environment details (Kubernetes version, ChaosPlane version, OS)
- Relevant logs or error messages

For security vulnerabilities, see [SECURITY.md](SECURITY.md) instead.

## Code of Conduct

All participants are expected to follow our [Code of Conduct](CODE_OF_CONDUCT.md).

## License

By contributing to ChaosPlane, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
