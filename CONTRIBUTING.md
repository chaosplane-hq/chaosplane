# Contributing to ChaosPlane

Thank you for your interest in contributing to ChaosPlane.

## Getting Started

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make your changes
4. Run tests: `make test && make lint`
5. Ensure generated files are up to date: `make generate manifests && git diff --exit-code`
6. Submit a pull request

## Code Style

- Go code follows [Effective Go](https://go.dev/doc/effective_go) and is enforced by `golangci-lint`
- Format with `gofumpt`: `make fmt`
- All exported types and functions must have doc comments

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation
- `refactor:` code refactoring
- `test:` adding or updating tests
- `chore:` maintenance

## Pull Requests

- PRs require at least 1 approving review
- CI must pass (lint, test, generate-check)
- Keep PRs focused — one feature or fix per PR

## Reporting Issues

Use GitHub Issues. Include:
- Steps to reproduce
- Expected vs actual behavior
- Environment details (K8s version, OS, etc.)

## Code of Conduct

See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
