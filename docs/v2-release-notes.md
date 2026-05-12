# ChaosPlane v2.0.0 Release Notes

> v2.0.0 is the culmination of 36 months of development across five phases. It ships 40+ chaos executor types, a full SaaS platform, AI-assisted analysis, enterprise compliance, multi-cloud support, multi-region deployment, and a redesigned API. This release marks ChaosPlane's graduation from open-source tool to enterprise-grade chaos engineering platform.
>
> Release date: Month 33-36 (Phase 5)
> API: v2 (v1 supported until v3.0.0 or 12 months after v2.0.0 GA, whichever is later)

---

## What's New in v2.0.0

### Architecture Improvements

The v2.0.0 architecture is a significant evolution from v0.1.0. The core operator has been redesigned for large-scale deployments.

**Multi-cluster federation**

ChaosPlane now manages chaos experiments across multiple Kubernetes clusters from a single control plane. The federation controller synchronizes experiment state across clusters and provides a unified view of resilience across your entire fleet.

```yaml
apiVersion: chaos.chaosplane.dev/v2
kind: FederatedChaosExperiment
metadata:
  name: cross-cluster-network-partition
spec:
  clusters:
    - name: prod-us-east
      selector:
        matchLabels:
          env: production
          region: us-east-1
    - name: prod-eu-west
      selector:
        matchLabels:
          env: production
          region: eu-west-1
  template:
    spec:
      action: network-partition
      duration: 5m
      selector:
        namespaces: [payments]
```

**Performance at scale**

v2.0.0 is validated against clusters with 1,000+ nodes. The operator's reconciliation loop has been redesigned with work queues and rate limiting to handle large experiment volumes without overwhelming the Kubernetes API server.

| Metric | v0.1.0 | v2.0.0 |
|---|---|---|
| Max cluster nodes | ~100 (tested) | 1,000+ (validated) |
| Concurrent experiments | 20 | 200+ |
| Experiment start latency (p99) | 8s | 2s |
| Operator memory (idle) | 120 MB | 80 MB |
| Operator memory (100 experiments) | 400 MB | 180 MB |

**Pluggable executor registry**

The executor interface has been stabilized as a public API. Third-party executors can now be distributed as OCI artifacts and loaded at runtime without recompiling the operator.

```go
// Executor interface v2 — stable public API
type Executor interface {
    Name() string
    Validate(spec json.RawMessage) error
    Execute(ctx context.Context, req ExecutionRequest) (ExecutionResult, error)
    Recover(ctx context.Context, req RecoveryRequest) error
    Status(ctx context.Context, req StatusRequest) (ExecutionStatus, error)
}
```

---

## API v2

API v2 is a clean redesign that addresses inconsistencies in v1 while maintaining backward compatibility for the most common operations.

### Breaking Changes from v1

| v1 endpoint | v2 endpoint | Change |
|---|---|---|
| `POST /api/v1/experiments` | `POST /api/v2/experiments` | Request body restructured |
| `GET /api/v1/experiments/{id}/status` | `GET /api/v2/experiments/{id}` (status in body) | Status merged into experiment resource |
| `POST /api/v1/workflows` | `POST /api/v2/workflows` | DAG spec format updated |
| `GET /api/v1/metrics` | `GET /api/v2/resilience/score` | Renamed for clarity |

v1 API remains available at `/api/v1/` and is supported until v3.0.0 or 12 months after v2.0.0 GA. A migration guide is available at https://docs.chaosplane.dev/migration/v1-to-v2.

### New v2 API Features

- Bulk experiment operations: create, abort, or delete multiple experiments in a single request
- Experiment templates: save and reuse experiment configurations via the API
- Resilience score API: query historical resilience scores with time-range filtering
- Webhook subscriptions: subscribe to experiment lifecycle events via HTTP webhooks
- GraphQL endpoint: `POST /api/v2/graphql` for flexible querying of experiment data

---

## Phase-by-Phase Feature Summary

### Phase 0 — Bootstrap (Month 0-3)

The foundation. Go module structure, CI/CD pipeline, security scanning, and the core operator skeleton.

- Go workspace with `chaosplane` (operator + CLI) and `chaosplane-platform` (SaaS) modules
- GitHub Actions CI: lint, test, build on every PR
- GoReleaser: multi-arch container builds, SBOM generation, cosign signing
- Security pipeline: Trivy, Semgrep, Gitleaks, govulncheck
- `ChaosExperiment` CRD v1alpha1 with 8-phase state machine
- Executor interface and registry pattern

### Phase 1 — OSS MVP (Month 3-9)

The first usable release. 20 chaos executors, the chaos-daemon, CLI, and basic platform.

**Chaos executors (20)**

Pod actions:
- `pod-kill` — terminates pods matching a selector
- `container-kill` — kills a specific container within a pod
- `pod-cpu-stress` — CPU pressure injection
- `pod-memory-stress` — memory pressure injection
- `pod-io-stress` — disk I/O pressure injection
- `pod-dns-error` — DNS resolution corruption inside a pod
- `pod-http-abort` — aborts outbound HTTP requests at a configurable rate
- `pod-http-delay` — adds latency to outbound HTTP requests

Network actions:
- `network-delay` — configurable latency injection
- `network-loss` — packet drop at configurable percentage
- `network-corrupt` — packet corruption in transit
- `network-duplicate` — packet duplication
- `network-partition` — pod isolation from peers
- `network-bandwidth` — bandwidth throttling

Node actions:
- `node-drain` — cordon and drain a Kubernetes node
- `node-taint` — apply a taint to trigger pod evictions
- `node-restart` — reboot a node via chaos-daemon
- `node-cpu-stress` — node-level CPU pressure

Stress actions:
- `stress-cpu` — CPU stressor process inside a container
- `stress-memory` — memory stressor process inside a container

**Operator features**
- `ChaosWorkflow` CRD with DAG-based workflow engine
- `BlastRadiusPolicy` CRD with validating admission webhook
- Steady-state probes: Prometheus, HTTP, Kubernetes
- Abort conditions with automatic rollback
- Time windows with cron-based scheduling

**chaos-daemon**
- gRPC server with 7 RPCs for node-level fault injection
- DaemonSet deployment, mTLS communication

**CLI (chaosctl)**
- 12 subcommands covering full experiment lifecycle
- Shell completion for bash, zsh, fish, PowerShell

**Platform**
- REST API (Gin) with full CRUD
- Next.js 15 + Carbon Design System web UI
- Real-time WebSocket updates

### Phase 2 — SaaS Platform (Month 9-15)

Multi-tenant SaaS, identity, billing, and the AI assistant.

**Multi-tenancy**
- Organization and team model with RBAC
- Row-level security for complete tenant data isolation
- SCIM provisioning for enterprise IdP integration
- SAML SSO (Okta, Azure AD, Google Workspace)

**Billing and plans**
- Stripe integration: Starter (free), Growth, Enterprise tiers
- Usage-based metering: experiments, nodes, AI calls
- Self-serve upgrade/downgrade
- Invoice management

**AI Assistant**
- Topology analysis: maps service dependencies from Kubernetes resources
- Vulnerability detection: identifies single points of failure and blast radius risks
- Experiment recommendations: suggests experiments based on topology and past results
- Result summarization: natural language summaries of experiment outcomes
- Predictive failure analysis: identifies patterns that precede failures

**Resilience scoring**
- Quantitative resilience score (0-100) per service and cluster
- Historical trend tracking
- Score breakdown by failure category (network, compute, storage, dependency)

**Additional executors (Phase 2, +8 = 28 total)**
- `aws-ec2-stop` — stop/terminate EC2 instances
- `aws-rds-failover` — trigger RDS Multi-AZ failover
- `aws-ecs-task-stop` — stop ECS tasks
- `aws-lambda-throttle` — throttle Lambda function concurrency
- `aws-s3-deny` — inject S3 access denial via bucket policy
- `aws-elb-latency` — inject latency at the load balancer level
- `aws-az-failure` — simulate availability zone failure
- `aws-security-group-block` — block traffic via security group modification

### Phase 3 — Enterprise (Month 15-21)

Enterprise features, compliance foundations, and government plan.

**Enterprise features**
- ABAC (Attribute-Based Access Control) for fine-grained permissions
- Audit logs: tamper-evident, append-only, 3-year retention
- Custom compliance reports: SOC 2, ISO 27001, FedRAMP control evidence
- Multi-cluster support: manage experiments across multiple clusters
- GameDay orchestration: structured resilience exercises with runbooks
- Blast radius visualization: real-time impact graph during experiments

**Compliance certifications achieved**
- SOC 2 Type II
- ISO 27001
- ISMS-P (Korea)
- CMMC Level 2

**Government plan**
- AWS GovCloud deployment (us-gov-west-1)
- FIPS 140-2 cryptography (BoringCrypto)
- PIV/CAC authentication via agency IdP
- CUI marking and handling
- Air-gap deployment bundle
- FedRAMP Moderate SSP drafted

**Additional executors (Phase 3, +6 = 34 total)**
- `vm-cpu-stress` — CPU stress on virtual machines (AWS EC2, Azure VM, GCP Compute)
- `vm-memory-stress` — memory stress on virtual machines
- `vm-disk-fill` — fill disk on virtual machines
- `vm-network-delay` — network latency on VM interfaces
- `vm-shutdown` — graceful or forced VM shutdown
- `vm-process-kill` — kill specific processes on VMs

### Phase 4 — Multi-Cloud (Month 21-24)

Azure and GCP chaos executors, eBPF-level fault injection, CI/CD integration.

**Azure chaos executors (+3 = 37 total)**
- `azure-vm-stop` — stop Azure Virtual Machines
- `azure-aks-node-drain` — drain AKS nodes
- `azure-cosmos-failover` — trigger Cosmos DB regional failover

**GCP chaos executors (+3 = 40 total)**
- `gcp-gke-node-drain` — drain GKE nodes
- `gcp-cloud-sql-failover` — trigger Cloud SQL failover
- `gcp-cloud-run-throttle` — throttle Cloud Run service concurrency

**eBPF executor framework**
- Kernel-level fault injection without modifying application code
- `ebpf-syscall-delay` — delay specific syscalls
- `ebpf-network-drop` — drop packets at the kernel network stack
- `ebpf-file-error` — inject file I/O errors at the kernel level

**CI/CD integration**
- GitHub Actions: `chaosplane/run-experiment` action
- GitLab CI: ChaosPlane executor for `.gitlab-ci.yml`
- Jenkins: ChaosPlane Jenkins plugin
- Automatic experiment execution on deployment, with pass/fail gate

```yaml
# GitHub Actions example
- name: Run chaos experiment
  uses: chaosplane/run-experiment@v2
  with:
    experiment: experiments/pod-kill-payments.yaml
    timeout: 10m
    fail-on-abort: true
  env:
    CHAOSPLANE_API_KEY: ${{ secrets.CHAOSPLANE_API_KEY }}
```

**Community marketplace**
- Custom chaos action marketplace (OCI artifact-based)
- Workflow template sharing
- Integration marketplace (monitoring, CI/CD, alerting)

### Phase 5 — Global (Month 24-36)

Multi-region, compliance completions, AWS Marketplace, CNCF Incubation.

**Multi-region deployment**
- US Commercial (us-east-1)
- EU with GDPR data residency (eu-west-1)
- APAC (ap-northeast-2 Seoul, ap-southeast-1 Singapore)
- GovCloud (us-gov-west-1)
- Cross-region management console (metadata only, no data movement)

**Compliance completions**
- FedRAMP Moderate ATO
- DoD IL4 authorization
- DoD IL5 authorization (dedicated infrastructure)
- CSAP 상등급 (Korea Advanced Grade)

**AWS Marketplace**
- PAYG and annual contract pricing
- Private Offer support
- AWS Partner Network (APN) ISV Accelerate registration

**v2.0.0 platform features**
- Multi-cluster federation with `FederatedChaosExperiment` CRD
- Cross-region GameDay orchestration
- Predictive failure analysis (AI-powered)
- Self-healing recommendations
- Custom compliance report builder
- Resilience score API v2 with historical trending

---

## Executor Reference — All 40 Types

### Kubernetes (20)

| Executor | Category | Description |
|---|---|---|
| `pod-kill` | Pod | Terminate pods matching selector |
| `container-kill` | Pod | Kill specific container in a pod |
| `pod-cpu-stress` | Pod | CPU pressure in pod container |
| `pod-memory-stress` | Pod | Memory pressure in pod container |
| `pod-io-stress` | Pod | Disk I/O pressure in pod container |
| `pod-dns-error` | Pod | DNS resolution corruption |
| `pod-http-abort` | Pod | Abort outbound HTTP requests |
| `pod-http-delay` | Pod | Latency on outbound HTTP requests |
| `network-delay` | Network | Configurable latency injection |
| `network-loss` | Network | Packet drop |
| `network-corrupt` | Network | Packet corruption |
| `network-duplicate` | Network | Packet duplication |
| `network-partition` | Network | Pod isolation from peers |
| `network-bandwidth` | Network | Bandwidth throttling |
| `node-drain` | Node | Cordon and drain node |
| `node-taint` | Node | Apply taint to trigger evictions |
| `node-restart` | Node | Reboot node via chaos-daemon |
| `node-cpu-stress` | Node | Node-level CPU pressure |
| `stress-cpu` | Stress | CPU stressor in container |
| `stress-memory` | Stress | Memory stressor in container |

### AWS (8)

| Executor | Description |
|---|---|
| `aws-ec2-stop` | Stop/terminate EC2 instances |
| `aws-rds-failover` | Trigger RDS Multi-AZ failover |
| `aws-ecs-task-stop` | Stop ECS tasks |
| `aws-lambda-throttle` | Throttle Lambda concurrency |
| `aws-s3-deny` | Inject S3 access denial |
| `aws-elb-latency` | Inject latency at load balancer |
| `aws-az-failure` | Simulate AZ failure |
| `aws-security-group-block` | Block traffic via security group |

### Virtual Machine (6)

| Executor | Description |
|---|---|
| `vm-cpu-stress` | CPU stress on VMs |
| `vm-memory-stress` | Memory stress on VMs |
| `vm-disk-fill` | Fill disk on VMs |
| `vm-network-delay` | Network latency on VM interfaces |
| `vm-shutdown` | Graceful or forced VM shutdown |
| `vm-process-kill` | Kill specific processes on VMs |

### Azure (3)

| Executor | Description |
|---|---|
| `azure-vm-stop` | Stop Azure Virtual Machines |
| `azure-aks-node-drain` | Drain AKS nodes |
| `azure-cosmos-failover` | Trigger Cosmos DB regional failover |

### GCP (3)

| Executor | Description |
|---|---|
| `gcp-gke-node-drain` | Drain GKE nodes |
| `gcp-cloud-sql-failover` | Trigger Cloud SQL failover |
| `gcp-cloud-run-throttle` | Throttle Cloud Run concurrency |

---

## Upgrade Guide

### From v1.x to v2.0.0

1. Back up all `ChaosExperiment` and `ChaosWorkflow` resources: `kubectl get chaosexperiments,chaosworkflows -A -o yaml > backup.yaml`
2. Update the Helm chart: `helm upgrade chaosplane chaosplane/chaosplane --version 2.0.0 -f values.yaml`
3. The operator will automatically migrate CRDs from v1alpha1 to v2alpha1
4. Update API clients from `/api/v1/` to `/api/v2/` (v1 remains available during transition)
5. Review breaking changes in the API v2 migration guide: https://docs.chaosplane.dev/migration/v1-to-v2
6. Update `chaosctl` to v2.0.0: `brew upgrade chaosctl` or download from GitHub Releases

### From v0.1.0 to v2.0.0

Direct upgrade from v0.1.0 is not supported. Upgrade to v1.0.0 first, then to v2.0.0. See the v1.0.0 upgrade guide for the v0.1.0 → v1.0.0 path.

---

## Known Issues

- `ebpf-syscall-delay` requires kernel 5.8+ with BTF enabled. Older kernels will receive a validation error at experiment creation time.
- `azure-cosmos-failover` requires the chaos agent to have Cosmos DB Operator role in the target Azure subscription.
- Cross-region GameDay is in beta. Feedback welcome via GitHub Discussions.

---

## Contributors

v2.0.0 was built by the ChaosPlane team and community contributors across 36 months. Full contributor list: https://github.com/chaosplane-hq/chaosplane/graphs/contributors

---

## Links

- Documentation: https://docs.chaosplane.dev
- Migration guide (v1 → v2): https://docs.chaosplane.dev/migration/v1-to-v2
- GitHub: https://github.com/chaosplane-hq/chaosplane
- Changelog: CHANGELOG.md
- Security: SECURITY.md
- Community Slack: CNCF Slack #chaosplane
