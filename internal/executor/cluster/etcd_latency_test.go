package cluster_test

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/cluster"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
	"google.golang.org/grpc"
)

type mockDaemon struct {
	netResp    *daemonv1.NetworkChaosResponse
	netErr     error
	cancelResp *daemonv1.CancelResponse
	lastNet    *daemonv1.NetworkChaosRequest
	lastCancel *daemonv1.CancelRequest
}

func (m *mockDaemon) ExecNetworkChaos(_ context.Context, in *daemonv1.NetworkChaosRequest, _ ...grpc.CallOption) (*daemonv1.NetworkChaosResponse, error) {
	m.lastNet = in
	return m.netResp, m.netErr
}
func (m *mockDaemon) ExecStressChaos(context.Context, *daemonv1.StressChaosRequest, ...grpc.CallOption) (*daemonv1.StressChaosResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockDaemon) ExecDNSChaos(context.Context, *daemonv1.DNSChaosRequest, ...grpc.CallOption) (*daemonv1.DNSChaosResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockDaemon) ExecHTTPChaos(context.Context, *daemonv1.HTTPChaosRequest, ...grpc.CallOption) (*daemonv1.HTTPChaosResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockDaemon) ExecNodeChaos(context.Context, *daemonv1.NodeChaosRequest, ...grpc.CallOption) (*daemonv1.NodeChaosResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockDaemon) CancelChaos(_ context.Context, in *daemonv1.CancelRequest, _ ...grpc.CallOption) (*daemonv1.CancelResponse, error) {
	m.lastCancel = in
	return m.cancelResp, nil
}
func (m *mockDaemon) GetChaosStatus(context.Context, *daemonv1.StatusRequest, ...grpc.CallOption) (*daemonv1.StatusResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func etcdPod(name, node string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "kube-system",
			Labels:    map[string]string{"component": "etcd"},
		},
		Spec: corev1.PodSpec{NodeName: node},
	}
}

func TestEtcdLatency_TargetsEtcdPodsByDefault(t *testing.T) {
	p := etcdPod("etcd-cp1", "cp1")
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	md := &mockDaemon{netResp: &daemonv1.NetworkChaosResponse{Success: true, ExecutionId: "exec-etcd-1"}}
	factory := func(string) (daemonv1.ChaosDaemonClient, error) { return md, nil }

	e := cluster.NewEtcdLatencyExecutor(testLogger(), c, pod.DaemonClientFactory(factory))
	exp := newExpNoNames("etcd-latency", map[string]string{"latency": "200ms"})

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if md.lastNet == nil || md.lastNet.Action != "delay" {
		t.Fatalf("expected delay network chaos, got %+v", md.lastNet)
	}
	if md.lastNet.PodName != "etcd-cp1" {
		t.Fatalf("expected etcd pod targeted, got %q", md.lastNet.PodName)
	}
}

func TestEtcdLatency_RollbackCancels(t *testing.T) {
	p := etcdPod("etcd-cp1", "cp1")
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	md := &mockDaemon{
		netResp:    &daemonv1.NetworkChaosResponse{Success: true, ExecutionId: "exec-etcd-1"},
		cancelResp: &daemonv1.CancelResponse{Success: true},
	}
	factory := func(string) (daemonv1.ChaosDaemonClient, error) { return md, nil }

	e := cluster.NewEtcdLatencyExecutor(testLogger(), c, pod.DaemonClientFactory(factory))
	exp := newExpNoNames("etcd-latency", map[string]string{"latency": "200ms"})

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if md.lastCancel == nil || md.lastCancel.ExecutionId != "exec-etcd-1" {
		t.Fatalf("expected cancel of exec-etcd-1, got %+v", md.lastCancel)
	}
}

func TestEtcdLatency_DaemonFailureSurfaces(t *testing.T) {
	p := etcdPod("etcd-cp1", "cp1")
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
	md := &mockDaemon{netResp: &daemonv1.NetworkChaosResponse{Success: false, Message: "no veth for hostNetwork etcd"}}
	factory := func(string) (daemonv1.ChaosDaemonClient, error) { return md, nil }

	e := cluster.NewEtcdLatencyExecutor(testLogger(), c, pod.DaemonClientFactory(factory))
	exp := newExpNoNames("etcd-latency", map[string]string{"latency": "200ms"})

	if err := e.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected daemon failure to surface (honest), got nil")
	}
}

func TestEtcdLatency_Validate(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme).Build()
	factory := func(string) (daemonv1.ChaosDaemonClient, error) { return &mockDaemon{}, nil }
	e := cluster.NewEtcdLatencyExecutor(testLogger(), c, pod.DaemonClientFactory(factory))

	if err := e.Validate(newExpNoNames("etcd-latency", map[string]string{"latency": "100ms"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if err := e.Validate(newExpNoNames("etcd-latency", map[string]string{})); err == nil {
		t.Fatal("expected error for missing latency")
	}
}
