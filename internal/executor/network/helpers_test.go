package network_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/network"
	"google.golang.org/grpc"
)

var testScheme = runtime.NewScheme()

func init() {
	_ = corev1.AddToScheme(testScheme)
	_ = v1alpha1.AddToScheme(testScheme)
}

type mockDaemonClient struct {
	networkResp *daemonv1.NetworkChaosResponse
	networkErr  error
	cancelResp  *daemonv1.CancelResponse
	cancelErr   error
	lastNetwork *daemonv1.NetworkChaosRequest
	lastCancel  *daemonv1.CancelRequest
}

func (m *mockDaemonClient) ExecNetworkChaos(_ context.Context, in *daemonv1.NetworkChaosRequest, _ ...grpc.CallOption) (*daemonv1.NetworkChaosResponse, error) {
	m.lastNetwork = in
	return m.networkResp, m.networkErr
}

func (m *mockDaemonClient) ExecStressChaos(_ context.Context, _ *daemonv1.StressChaosRequest, _ ...grpc.CallOption) (*daemonv1.StressChaosResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockDaemonClient) ExecDNSChaos(_ context.Context, _ *daemonv1.DNSChaosRequest, _ ...grpc.CallOption) (*daemonv1.DNSChaosResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockDaemonClient) ExecHTTPChaos(_ context.Context, _ *daemonv1.HTTPChaosRequest, _ ...grpc.CallOption) (*daemonv1.HTTPChaosResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockDaemonClient) ExecNodeChaos(_ context.Context, _ *daemonv1.NodeChaosRequest, _ ...grpc.CallOption) (*daemonv1.NodeChaosResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockDaemonClient) CancelChaos(_ context.Context, in *daemonv1.CancelRequest, _ ...grpc.CallOption) (*daemonv1.CancelResponse, error) {
	m.lastCancel = in
	return m.cancelResp, m.cancelErr
}

func (m *mockDaemonClient) GetChaosStatus(_ context.Context, _ *daemonv1.StatusRequest, _ ...grpc.CallOption) (*daemonv1.StatusResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func mockFactory(mc *mockDaemonClient) network.DaemonClientFactory {
	return func(_ string) (daemonv1.ChaosDaemonClient, error) {
		return mc, nil
	}
}

func failingFactory() network.DaemonClientFactory {
	return func(_ string) (daemonv1.ChaosDaemonClient, error) {
		return nil, fmt.Errorf("connection refused")
	}
}

func newExp(action string, params map[string]string, targetNames ...string) *v1alpha1.ChaosExperiment {
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-exp",
			Namespace: "default",
			UID:       types.UID("test-uid-123"),
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Action: v1alpha1.ActionSpec{Type: action},
			Target: v1alpha1.TargetSpec{
				Kind:      "Pod",
				Namespace: "default",
				Names:     targetNames,
			},
		},
	}
	if params != nil {
		raw, _ := json.Marshal(params)
		exp.Spec.Action.Parameters = runtime.RawExtension{Raw: raw}
	}
	return exp
}

func testPod(name, namespace, nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}
