# ChaosPlane v0.1.0 Security Audit Report

**Date:** 2026-04-10
**Auditor:** Automated static analysis + manual code review
**Scope:** chaosplane (operator, daemon, CLI), chaosplane-platform (API server, web app)
**Classification:** Internal

---

## Executive Summary

The ChaosPlane v0.1.0 codebase demonstrates solid security fundamentals: distroless container images running as non-root, mTLS support for daemon gRPC communication, admission webhook policy enforcement, and proper use of parameterized queries (no SQL injection vectors). No hardcoded secrets or leaked credentials were found.

**0 Critical findings. 3 High findings. 3 Medium findings. 4 Low/Informational findings.**

All HIGH findings have documented remediation plans and are tracked for resolution before v0.2.0.

---

## Scope

| Component | Path | Languages |
|-----------|------|-----------|
| Operator (controller, webhook) | `chaosplane/cmd/operator`, `chaosplane/internal/controller`, `chaosplane/internal/webhook` | Go |
| Daemon (gRPC server) | `chaosplane/cmd/daemon`, `chaosplane/internal/daemon` | Go |
| Executors (pod, network, node, stress) | `chaosplane/internal/executor/` | Go |
| Probes (HTTP, Prometheus, K8s) | `chaosplane/internal/probe/` | Go |
| CLI (chaosctl) | `chaosplane/cmd/chaosctl`, `chaosplane/internal/cli` | Go |
| Platform API | `chaosplane-platform/apps/api` | Go |
| Platform Web | `chaosplane-platform/apps/web` | TypeScript/React |
| Migrations | `chaosplane-platform/migrations/` | SQL |

## Methodology

1. Manual source code review of all security-sensitive components
2. Grep-based pattern scanning for secrets, credentials, unsafe patterns
3. Dependency analysis via `go.mod` review
4. Dockerfile security review
5. gRPC transport security review
6. Webhook admission control review
7. RBAC configuration review

---

## Findings

### Critical

None.

### High

#### H-1: Insecure gRPC Transport in Executor DefaultDaemonClientFactory

**Severity:** HIGH
**CWE:** CWE-319 (Cleartext Transmission of Sensitive Information)
**Location:**
- `internal/executor/pod/common.go:31`
- `internal/executor/node/common.go:31`
- `internal/executor/stress/common.go:37`

**Description:** All three executor packages define a `DefaultDaemonClientFactory` that connects to the chaos daemon using `insecure.NewCredentials()`. While the daemon binary (`cmd/daemon/main.go`) supports mTLS via `--tls-cert`, `--tls-key`, `--tls-ca` flags, the operator-side executor clients hardcode insecure transport. This means operator-to-daemon gRPC traffic (containing chaos execution commands) travels in plaintext within the cluster.

**Risk:** An attacker with network access within the cluster could intercept or inject chaos commands.

**Remediation:**
1. Inject TLS credentials into `DaemonClientFactory` from operator configuration
2. Make mTLS the default; fail-open only when explicitly configured for development
3. Load CA cert from a mounted Secret or ServiceAccount projected volume

**Timeline:** v0.2.0

---

#### H-2: WebSocket Authentication Token in URL Query Parameter

**Severity:** HIGH
**CWE:** CWE-598 (Use of GET Request Method With Sensitive Query Strings)
**Location:** `chaosplane-platform/apps/api/internal/handler/websocket.go:35`

**Description:** The WebSocket endpoint accepts the authentication token as a URL query parameter (`?token=...`). Tokens in URLs are logged by proxies, load balancers, browser history, and server access logs.

**Risk:** Token leakage through infrastructure logs, enabling session hijacking.

**Remediation:**
1. Use the `Sec-WebSocket-Protocol` subprotocol header for token transport
2. Alternatively, require an initial HTTP-based token exchange that returns a short-lived WebSocket ticket
3. Ensure access logs redact query parameters containing tokens

**Timeline:** v0.2.0

---

#### H-3: WebSocket Wildcard Origin

**Severity:** HIGH
**CWE:** CWE-346 (Origin Validation Error)
**Location:** `chaosplane-platform/apps/api/internal/handler/websocket.go:42`

**Description:** The WebSocket `AcceptOptions` uses `OriginPatterns: []string{"*"}`, accepting connections from any origin. This disables the browser's same-origin protection for WebSocket connections.

**Risk:** Cross-site WebSocket hijacking — a malicious page could open a WebSocket to the API and receive experiment status data using the victim's credentials.

**Remediation:**
1. Restrict `OriginPatterns` to the known frontend domain(s)
2. Make the allowed origins configurable via environment variable

**Timeline:** v0.2.0

---

### Medium

#### M-1: Unbounded io.ReadAll in Probes

**Severity:** MEDIUM
**CWE:** CWE-400 (Uncontrolled Resource Consumption)
**Location:**
- `internal/probe/http.go:44` — `io.ReadAll(resp.Body)`
- `internal/probe/prometheus.go:51` — `io.ReadAll(resp.Body)`

**Description:** HTTP and Prometheus probes read the entire response body into memory without size limits. A malicious or misconfigured probe target could return an arbitrarily large response, causing OOM in the operator.

**Remediation:** Wrap with `io.LimitReader(resp.Body, maxBytes)` (e.g., 1 MB limit).

**Timeline:** v0.2.0

---

#### M-2: No gRPC Request Size Limits on Daemon Server

**Severity:** MEDIUM
**CWE:** CWE-400 (Uncontrolled Resource Consumption)
**Location:** `cmd/daemon/main.go:43`

**Description:** The gRPC server is created with `grpc.NewServer(opts...)` without `grpc.MaxRecvMsgSize()` or `grpc.MaxSendMsgSize()` options. The default gRPC max message size is 4 MB, which is reasonable, but should be explicitly set for defense-in-depth.

**Remediation:** Add explicit `grpc.MaxRecvMsgSize(4 << 20)` to the server options.

**Timeline:** v0.2.0

---

#### M-3: Daemon Runs Without mTLS by Default

**Severity:** MEDIUM
**CWE:** CWE-319 (Cleartext Transmission of Sensitive Information)
**Location:** `cmd/daemon/main.go:33`

**Description:** The daemon only enables TLS when all three flags (`--tls-cert`, `--tls-key`, `--tls-ca`) are provided. If any flag is missing, the server silently falls back to plaintext. There is no warning log when running without TLS.

**Remediation:**
1. Log a WARNING when TLS flags are not provided
2. Consider requiring TLS by default with an explicit `--insecure` flag for development

**Timeline:** v0.2.0

---

### Low / Informational

#### L-1: Hardcoded Daemon Port (9090)

**Severity:** LOW
**Location:** `internal/executor/pod/common.go:19`, `internal/executor/node/common.go:19`, `internal/executor/stress/common.go:19`

**Description:** The daemon gRPC port is hardcoded to 9090 in all executor packages. This should be configurable.

**Remediation:** Make the port configurable via operator flags or environment variable.

---

#### L-2: gRPC Connection Not Closed After Use

**Severity:** LOW
**CWE:** CWE-404 (Improper Resource Shutdown)
**Location:** `internal/executor/pod/common.go:31`, `internal/executor/node/common.go:31`, `internal/executor/stress/common.go:37`

**Description:** `DefaultDaemonClientFactory` creates a new gRPC connection per call but never closes it. Over time this could leak file descriptors.

**Remediation:** Return a closer or use a connection pool.

---

#### L-3: Regex Compilation on Every HTTP Probe Run

**Severity:** INFO
**Location:** `internal/probe/http.go:48`

**Description:** `regexp.MatchString` compiles the regex on every probe invocation. For probes that run repeatedly, this is wasteful.

**Remediation:** Pre-compile the regex in the probe constructor.

---

#### L-4: No Rate Limiting on Webhook Endpoint

**Severity:** INFO
**Location:** `cmd/operator/main.go:104`

**Description:** The admission webhook has no rate limiting. A flood of ChaosExperiment creation requests could overwhelm the policy evaluation logic.

**Remediation:** Rely on Kubernetes API server admission webhook timeout configuration (default 10s) and consider adding a simple rate limiter if needed.

---

## Dependency Analysis

### Direct Dependencies (go.mod)

| Dependency | Version | Status |
|-----------|---------|--------|
| google.golang.org/grpc | v1.80.0 | Current, no known CVEs |
| k8s.io/api | v0.32.1 | Current |
| k8s.io/apimachinery | v0.32.3 | Current |
| k8s.io/client-go | v0.32.1 | Current |
| sigs.k8s.io/controller-runtime | v0.20.4 | Current |
| github.com/spf13/cobra | v1.10.2 | Current |
| google.golang.org/protobuf | v1.36.11 | Current |
| golang.org/x/net | v0.49.0 | Current |

No known critical or high CVEs in direct or transitive dependencies at time of audit. The `govulncheck` CI job will provide ongoing monitoring.

### Notable Transitive Dependencies

- `github.com/golang/protobuf v1.5.4` — deprecated in favor of `google.golang.org/protobuf`, but still safe
- `github.com/gogo/protobuf v1.3.2` — pulled by k8s libraries, no active CVEs

---

## Container Image Security

All four Dockerfiles follow security best practices:

| Image | Base (build) | Base (runtime) | Non-root | Distroless | Stripped Binary |
|-------|-------------|----------------|----------|------------|-----------------|
| Dockerfile.operator | golang:1.24-alpine | gcr.io/distroless/static-debian12:nonroot | ✅ (65532) | ✅ | ✅ (-s -w) |
| Dockerfile.daemon | golang:1.24-alpine | gcr.io/distroless/static-debian12:nonroot | ✅ (65532) | ✅ | ✅ (-s -w) |
| Dockerfile.agent | golang:1.24-alpine | gcr.io/distroless/static-debian12:nonroot | ✅ (65532) | ✅ | ✅ (-s -w) |
| Dockerfile.chaosctl | golang:1.24-alpine | gcr.io/distroless/static-debian12:nonroot | ✅ (65532) | ✅ | ✅ (-s -w) |

**Positive findings:**
- Multi-stage builds — build tools not present in runtime image
- `CGO_ENABLED=0` — static binaries, no libc dependency
- `USER 65532:65532` — non-root execution
- Distroless base — minimal attack surface, no shell

**Noted risk (documented):**
- Daemon container requires `CAP_NET_ADMIN` at runtime for network fault injection. This is inherent to the chaos engineering use case and is documented in `Dockerfile.daemon`.

---

## RBAC Review

The `config/rbac/` directory is currently empty. RBAC manifests are likely generated or managed via controller-runtime/kustomize.

**Operator permissions (inferred from code):**
- ChaosExperiment, ChaosWorkflow, BlastRadiusPolicy: full CRUD + status updates
- Pods: get, list, delete (for pod-kill executor)
- Nodes: get, list, update (for node-drain, node-taint)
- Events: create, patch

**Recommendation:** Generate and commit explicit RBAC manifests with least-privilege ClusterRole definitions. Avoid wildcard verbs or resource groups.

---

## Network Security

### gRPC (Daemon Communication)

- **TLS support:** ✅ mTLS implemented in `internal/daemon/tls.go`
- **Min TLS version:** TLS 1.2 (`tls.VersionTLS12`) ✅
- **Client auth:** `tls.RequireAndVerifyClientCert` ✅
- **Issue:** Operator-side clients default to insecure transport (see H-1)

### Webhook (Admission Control)

- Registered via controller-runtime webhook server (TLS managed by cert-manager or controller-runtime)
- Validates ChaosExperiment against BlastRadiusPolicy before admission
- Supports both `enforce` and `audit` modes
- No bypass vectors found — all experiments must pass through the webhook

### Platform API (WebSocket)

- Token-based auth (see H-2 for query param concern)
- Connection limiter implemented (`maxSubscriptionsPerConn = 10`)
- Wildcard origin issue (see H-3)

---

## Secret Scanning Results

| Check | Result |
|-------|--------|
| Hardcoded passwords/secrets in Go source | ✅ None found |
| Hardcoded API keys/tokens | ✅ None found |
| .env files committed | ✅ None found |
| PEM/key files committed | ✅ None found |
| exec.Command with user input | ✅ None found (no exec.Command usage) |
| SQL injection vectors | ✅ None found (platform uses migrations, no dynamic SQL) |
| Path traversal | ✅ None found |

---

## Recommendations

1. **Enforce mTLS for operator-to-daemon communication** — highest priority, closes the most significant attack surface (H-1)
2. **Fix WebSocket auth token transport** — move from query param to header-based (H-2)
3. **Restrict WebSocket origins** — replace wildcard with configured domains (H-3)
4. **Add io.LimitReader to probes** — prevent OOM from malicious probe targets (M-1)
5. **Log warning when daemon runs without TLS** — operational visibility (M-3)
6. **Generate and commit RBAC manifests** — ensure least-privilege is auditable
7. **Run `govulncheck` in CI** — now configured in `.github/workflows/security.yaml`
8. **Enable Dependabot or Renovate** — automated dependency update PRs

---

## Remediation Timeline

| ID | Severity | Finding | Target |
|----|----------|---------|--------|
| H-1 | HIGH | Insecure gRPC transport in executors | v0.2.0 |
| H-2 | HIGH | WebSocket token in URL query param | v0.2.0 |
| H-3 | HIGH | WebSocket wildcard origin | v0.2.0 |
| M-1 | MEDIUM | Unbounded io.ReadAll in probes | v0.2.0 |
| M-2 | MEDIUM | No explicit gRPC message size limit | v0.2.0 |
| M-3 | MEDIUM | Daemon silent fallback to plaintext | v0.2.0 |
| L-1 | LOW | Hardcoded daemon port | v0.3.0 |
| L-2 | LOW | gRPC connection leak | v0.3.0 |
| L-3 | INFO | Regex recompilation in probe | v0.3.0 |
| L-4 | INFO | No webhook rate limiting | Backlog |
