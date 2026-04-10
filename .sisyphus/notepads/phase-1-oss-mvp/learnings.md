## T1-02: Pod Chaos Executors
- Generated gRPC code didn't include `NewChaosDaemonClient` — had to implement it manually in `common.go` wrapping `grpc.ClientConnInterface.Invoke`
- DaemonClientFactory pattern (func type) works well for DI in tests — mock client injected via factory closure
- `kubernetes.Interface` needed for pod-kill (client-go Delete), `sigs.k8s.io/controller-runtime/pkg/client.Client` for pod resolution (List/Get)
- `runtime.RawExtension` params unmarshaled to `map[string]string` via `ParseParameters` helper
- `nodeExecution` struct tracks per-pod execution IDs for rollback on stress/dns/http executors
- container-kill and pod-kill have no-op rollback (K8s auto-restarts)
- fake client from `sigs.k8s.io/controller-runtime/pkg/client/fake` works with `WithObjects()` for seeding test pods
- `k8s.io/client-go/kubernetes/fake.NewSimpleClientset` needed separately for pod-kill tests (Delete via clientset)

## T1-14: AbortConditions
- AbortConditionSpec reuses existing ProbeType/PrometheusProbe/HTTPProbe/K8sProbe — just wraps with Name+Action
- `abortConditionToProbeSpec` converts AbortConditionSpec→ProbeSpec for reuse with probe.NewProbe
- OR evaluation: iterate conditions, first triggered wins
- Requeue 5s when abort conditions exist (instead of full remaining duration)
- pause action treated as abort (no Paused phase in Phase 1)
- Tests skip gracefully without envtest (cfg==nil guard)

## T1-06: Stress Executors + Main Registration
- stress package mirrors node package structure: own DaemonClientFactory type, own common.go with ResolveTargetNodes/ParseParameters/gRPC client impl
- node.DaemonClientFactory and pod.DaemonClientFactory are distinct types with same signature — explicit type conversion needed in main.go: `node.DaemonClientFactory(node.DefaultDaemonClientFactory)`
- stress-cpu uses StressorType="cpu" with params: workers, load, duration
- stress-memory uses StressorType="memory" with params: workers, size, duration
- Test helpers go in helpers_test.go (same pattern as node package)
- main.go now registers 20 executors: 8 pod + 6 network + 4 node + 2 stress

## T1-16: Release Pipeline
- GoReleaser v2 config uses `version: 2` at top level
- `dockers` section needs separate entries per arch (amd64/arm64) with buildx, each producing `<image>:<tag>-<arch>` tags
- `docker_manifests` creates multi-arch manifest lists combining the per-arch images
- Release Dockerfiles under `build/release/` are minimal (distroless + COPY binary) — GoReleaser handles the Go build and copies the binary into the Docker build context
- cosign keyless signing uses `--yes` flag (replaces deprecated COSIGN_EXPERIMENTAL for CLI, but env var still needed for GHA OIDC)
- `anchore/sbom-action/download-syft@v0` installs syft for GoReleaser's `sboms` section
- Helm OCI push: `helm push <chart>.tgz oci://ghcr.io/<org>/helm-charts` — simpler than GitHub Pages approach
- helm-release job extracts semver from tag via `${GITHUB_REF_NAME#v}` parameter expansion
