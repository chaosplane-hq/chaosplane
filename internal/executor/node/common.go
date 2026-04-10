package node

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const daemonPort = 9090

// DaemonClientFactory creates daemon gRPC clients for a given endpoint.
type DaemonClientFactory func(endpoint string) (daemonv1.ChaosDaemonClient, error)

// ResolveDaemonEndpoint returns the daemon gRPC address for a node.
func ResolveDaemonEndpoint(nodeName string) string {
	return fmt.Sprintf("%s:%d", nodeName, daemonPort)
}

// DefaultDaemonClientFactory connects to the daemon via gRPC with insecure credentials.
func DefaultDaemonClientFactory(endpoint string) (daemonv1.ChaosDaemonClient, error) {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon at %s: %w", endpoint, err)
	}
	return newChaosDaemonClient(conn), nil
}

// ResolveTargetNodes resolves target nodes from the experiment's target spec.
func ResolveTargetNodes(ctx context.Context, k8sClient client.Client, target v1alpha1.TargetSpec) ([]corev1.Node, error) {
	if len(target.Names) > 0 {
		var nodes []corev1.Node
		for _, name := range target.Names {
			var n corev1.Node
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, &n); err != nil {
				return nil, fmt.Errorf("failed to get node %s: %w", name, err)
			}
			nodes = append(nodes, n)
		}
		return nodes, nil
	}

	if target.LabelSelector != nil {
		sel, err := metav1.LabelSelectorAsSelector(target.LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector: %w", err)
		}
		var nodeList corev1.NodeList
		if err := k8sClient.List(ctx, &nodeList,
			client.MatchingLabelsSelector{Selector: sel},
		); err != nil {
			return nil, fmt.Errorf("failed to list nodes: %w", err)
		}
		if len(nodeList.Items) == 0 {
			return nil, fmt.Errorf("no nodes matched selector %s", labels.Set(target.LabelSelector.MatchLabels))
		}
		return nodeList.Items, nil
	}

	return nil, fmt.Errorf("target must specify either names or labelSelector")
}

// ParseParameters unmarshals action parameters from RawExtension into a string map.
func ParseParameters(exp *v1alpha1.ChaosExperiment) (map[string]string, error) {
	params := make(map[string]string)
	if exp.Spec.Action.Parameters.Raw == nil {
		return params, nil
	}
	if err := json.Unmarshal(exp.Spec.Action.Parameters.Raw, &params); err != nil {
		return nil, fmt.Errorf("failed to parse action parameters: %w", err)
	}
	return params, nil
}

type chaosDaemonClient struct {
	cc grpc.ClientConnInterface
}

func newChaosDaemonClient(cc grpc.ClientConnInterface) daemonv1.ChaosDaemonClient {
	return &chaosDaemonClient{cc: cc}
}

func (c *chaosDaemonClient) ExecNetworkChaos(ctx context.Context, in *daemonv1.NetworkChaosRequest, opts ...grpc.CallOption) (*daemonv1.NetworkChaosResponse, error) {
	out := new(daemonv1.NetworkChaosResponse)
	err := c.cc.Invoke(ctx, "/"+daemonv1.ChaosDaemon_ServiceName+"/ExecNetworkChaos", in, out, opts...)
	return out, err
}

func (c *chaosDaemonClient) ExecStressChaos(ctx context.Context, in *daemonv1.StressChaosRequest, opts ...grpc.CallOption) (*daemonv1.StressChaosResponse, error) {
	out := new(daemonv1.StressChaosResponse)
	err := c.cc.Invoke(ctx, "/"+daemonv1.ChaosDaemon_ServiceName+"/ExecStressChaos", in, out, opts...)
	return out, err
}

func (c *chaosDaemonClient) ExecDNSChaos(ctx context.Context, in *daemonv1.DNSChaosRequest, opts ...grpc.CallOption) (*daemonv1.DNSChaosResponse, error) {
	out := new(daemonv1.DNSChaosResponse)
	err := c.cc.Invoke(ctx, "/"+daemonv1.ChaosDaemon_ServiceName+"/ExecDNSChaos", in, out, opts...)
	return out, err
}

func (c *chaosDaemonClient) ExecHTTPChaos(ctx context.Context, in *daemonv1.HTTPChaosRequest, opts ...grpc.CallOption) (*daemonv1.HTTPChaosResponse, error) {
	out := new(daemonv1.HTTPChaosResponse)
	err := c.cc.Invoke(ctx, "/"+daemonv1.ChaosDaemon_ServiceName+"/ExecHTTPChaos", in, out, opts...)
	return out, err
}

func (c *chaosDaemonClient) ExecNodeChaos(ctx context.Context, in *daemonv1.NodeChaosRequest, opts ...grpc.CallOption) (*daemonv1.NodeChaosResponse, error) {
	out := new(daemonv1.NodeChaosResponse)
	err := c.cc.Invoke(ctx, "/"+daemonv1.ChaosDaemon_ServiceName+"/ExecNodeChaos", in, out, opts...)
	return out, err
}

func (c *chaosDaemonClient) CancelChaos(ctx context.Context, in *daemonv1.CancelRequest, opts ...grpc.CallOption) (*daemonv1.CancelResponse, error) {
	out := new(daemonv1.CancelResponse)
	err := c.cc.Invoke(ctx, "/"+daemonv1.ChaosDaemon_ServiceName+"/CancelChaos", in, out, opts...)
	return out, err
}

func (c *chaosDaemonClient) GetChaosStatus(ctx context.Context, in *daemonv1.StatusRequest, opts ...grpc.CallOption) (*daemonv1.StatusResponse, error) {
	out := new(daemonv1.StatusResponse)
	err := c.cc.Invoke(ctx, "/"+daemonv1.ChaosDaemon_ServiceName+"/GetChaosStatus", in, out, opts...)
	return out, err
}
