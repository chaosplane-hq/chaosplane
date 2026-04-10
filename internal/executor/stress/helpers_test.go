package stress_test

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
	"github.com/chaosplane-hq/chaosplane/internal/executor/stress"
	"google.golang.org/grpc"
)

var testScheme = runtime.NewScheme()

func init() {
	_ = corev1.AddToScheme(testScheme)
	_ = v1alpha1.AddToScheme(testScheme)
}

type mockDaemonClient struct {
	stressResp *daemonv1.StressChaosResponse
	stressErr  error
	cancelResp *daemonv1.CancelResponse
	cancelErr  error
	lastStress *daemonv1.StressChaosRequest
	lastCancel *daemonv1.CancelRequest
}

func (m *mockDaemonClient) ExecNetworkChaos(_ context.Context, _ *daemonv1.NetworkChaosRequest, _ ...grpc.CallOption) (*daemonv1.NetworkChaosResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockDaemonClient) ExecStressChaos(_ context.Context, in *daemonv1.StressChaosRequest, _ ...grpc.CallOption) (*daemonv1.StressChaosResponse, error) {
	m.lastStress = in
	return m.stressResp, m.stressErr
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

func mockFactory(mc *mockDaemonClient) stress.DaemonClientFactory {
	return func(_ string) (daemonv1.ChaosDaemonClient, error) {
		return mc, nil
	}
}

func failingFactory() stress.DaemonClientFactory {
	return func(_ string) (daemonv1.ChaosDaemonClient, error) {
		return nil, fmt.Errorf("connection refused")
	}
}

func newStressExp(action string, params map[string]string, nodeNames ...string) *v1alpha1.ChaosExperiment {
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-exp",
			Namespace: "default",
			UID:       types.UID("test-uid-123"),
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Action: v1alpha1.ActionSpec{Type: action},
			Target: v1alpha1.TargetSpec{
				Kind:  "Node",
				Names: nodeNames,
			},
		},
	}
	if params != nil {
		raw, _ := json.Marshal(params)
		exp.Spec.Action.Parameters = runtime.RawExtension{Raw: raw}
	}
	return exp
}

func newStressExpWithSelector(action string, params map[string]string, matchLabels map[string]string) *v1alpha1.ChaosExperiment {
	exp := newStressExp(action, params)
	exp.Spec.Target.Names = nil
	exp.Spec.Target.LabelSelector = &metav1.LabelSelector{MatchLabels: matchLabels}
	return exp
}

func testNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"role": "worker"},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}
