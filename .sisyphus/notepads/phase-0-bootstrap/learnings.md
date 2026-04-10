
## T0-10 / T0-13: Daemon gRPC + chaosctl CLI + Agent placeholder

- Manual gRPC stubs in `gen/daemon/v1/` work fine without buf generate — define message types in `daemon.pb.go` and service interface + handlers in `daemon_grpc.pb.go`
- grpc health check uses `google.golang.org/grpc/health` and `grpc_health_v1` packages
- Cobra CLI: root.go init() wires subcommands, version uses ldflags pattern with `-X main.version=...`
- `go vet ./...` initially showed a transient error in controller code that resolved on re-run (likely stale cache after dependency upgrades)
- Dependencies added: google.golang.org/grpc, github.com/spf13/cobra (upgraded from v1.8.1 to v1.10.2)

## T0-09/09a/09b: Controller, Webhook, Executor Skeleton

- `ctrl.Reconciler` is not directly exported in controller-runtime v0.20.4 — avoid compile-time interface assertions against it
- `go mod tidy` needs to run twice sometimes when transitive deps pull in new modules (go-cmp case)
- `record.EventRecorder` comes from `k8s.io/client-go/tools/record` — gets pulled in transitively via controller-runtime
- envtest `ErrorIfCRDPathMissing: false` allows tests to compile/run even without CRD YAML manifests generated yet
- Webhook registration: `webhookServer.Register(path, webhook)` — no need for `admission.WithCustomValidator` when implementing `admission.Handler` directly
- fakeRecorder pattern: implement `record.EventRecorder` interface with no-ops for unit tests without full manager
- controller-runtime v0.20.4 uses `k8s.io/client-go v0.32.1` (not v0.32.3) — tidy resolves this automatically
