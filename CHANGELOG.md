# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-04-10

### Added

#### Chaos Actions (20 total)

**Pod actions (8)**
- `pod-kill` — terminates one or more pods matching a selector
- `container-kill` — kills a specific container within a running pod
- `pod-cpu-stress` — injects CPU pressure into a pod's container
- `pod-memory-stress` — injects memory pressure into a pod's container
- `pod-io-stress` — injects disk I/O pressure into a pod's container
- `pod-dns-error` — corrupts DNS resolution inside a pod
- `pod-http-abort` — aborts outbound HTTP requests at a configurable rate
- `pod-http-delay` — adds latency to outbound HTTP requests

**Network actions (6)**
- `network-delay` — adds configurable latency to network traffic
- `network-loss` — drops packets at a configurable percentage
- `network-corrupt` — corrupts packets in transit
- `network-duplicate` — duplicates packets on an interface
- `network-partition` — isolates a pod from a set of peers
- `network-bandwidth` — throttles network bandwidth on an interface

**Node actions (4)**
- `node-drain` — cordons and drains a Kubernetes node
- `node-taint` — applies a taint to a node to trigger pod evictions
- `node-restart` — reboots a node via the chaos-daemon
- `node-cpu-stress` — injects CPU pressure at the node level

**Stress actions (2)**
- `stress-cpu` — runs a CPU stressor process inside a target container
- `stress-memory` — runs a memory stressor process inside a target container

#### Operator

- `ChaosExperiment` CRD (`chaos.chaosplane.dev/v1alpha1`) with an 8-phase state machine: Pending, SteadyStateChecking, Running, Completing, Recovering, Completed, Failed, Aborted
- `ChaosWorkflow` CRD with a DAG-based workflow engine supporting experiment, delay, condition, parallel, and suspend template types
- `BlastRadiusPolicy` CRD with a validating admission webhook that enforces blast-radius constraints before experiments run
- Executor interface and registry pattern for pluggable, extensible chaos action implementations
- Steady-state probes (Prometheus, HTTP, Kubernetes) with pre-chaos and post-chaos validation phases
- Abort conditions with automatic rollback when a metric threshold is breached mid-experiment
- Time windows with cron-based allowed/blocked scheduling and full timezone support
- Configurable error-handling strategies: `abort`, `continue`, `retry`, `rollback`

#### chaos-daemon

- gRPC server exposing 7 RPCs for node-level fault injection
- DaemonSet deployment model so every node has a local daemon for network, node, and stress chaos
- Mutual TLS support for secure communication between the operator and daemon

#### CLI (chaosctl)

- 12 subcommands: `create`, `get`, `describe`, `delete`, `abort`, `logs`, `events`, `status`, `version`, `completion`, `workflow create`, `workflow get`
- Kubeconfig-aware client with namespace and context flags
- Shell completion for bash, zsh, fish, and PowerShell

#### Platform

- REST API server (Gin) with full CRUD endpoints for experiments and policies
- Web UI dashboard built with Next.js 15 and Carbon Design System, including experiment list, detail, create, and policy pages
- Real-time WebSocket updates so the dashboard reflects live experiment status without polling

#### Infrastructure

- Helm chart for single-command cluster installation with configurable values
- GoReleaser configuration for multi-arch container builds with SBOM generation and cosign signing
- GitHub Actions CI pipeline covering lint, test, and build on every pull request
- GitHub Actions release pipeline with automated Helm chart publishing to an OCI registry
- Security scanning pipeline integrating Trivy, Semgrep, Gitleaks, and govulncheck

#### Documentation

- Docusaurus documentation site with 39 pages
- Getting started guide, full action reference (20 pages), CRD reference, CLI reference, and architecture docs
- Local full-text search via docusaurus-search-local

#### Testing and Quality

- Unit tests across all packages
- End-to-end test framework with 20 executor test cases covering the full action surface
- Security audit report with findings and remediation plans
- 10 custom Semgrep rules for Go security patterns and Kubernetes operator best practices

[Unreleased]: https://github.com/chaosplane-hq/chaosplane/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/chaosplane-hq/chaosplane/releases/tag/v0.1.0
