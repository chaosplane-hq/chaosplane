## T1-02: Pod Chaos Executors
- Generated gRPC code didn't include `NewChaosDaemonClient` — had to implement it manually in `common.go` wrapping `grpc.ClientConnInterface.Invoke`
- DaemonClientFactory pattern (func type) works well for DI in tests — mock client injected via factory closure
- `kubernetes.Interface` needed for pod-kill (client-go Delete), `sigs.k8s.io/controller-runtime/pkg/client.Client` for pod resolution (List/Get)
- `runtime.RawExtension` params unmarshaled to `map[string]string` via `ParseParameters` helper
- `nodeExecution` struct tracks per-pod execution IDs for rollback on stress/dns/http executors
- container-kill and pod-kill have no-op rollback (K8s auto-restarts)
- fake client from `sigs.k8s.io/controller-runtime/pkg/client/fake` works with `WithObjects()` for seeding test pods
- `k8s.io/client-go/kubernetes/fake.NewSimpleClientset` needed separately for pod-kill tests (Delete via clientset)
