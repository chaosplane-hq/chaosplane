package node

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

var _ executor.Executor = (*RestartExecutor)(nil)

type RestartExecutor struct {
	Logger        *slog.Logger
	Client        client.Client
	DaemonFactory DaemonClientFactory

	mu           sync.Mutex
	executionIDs map[string][]nodeExecution
}

type nodeExecution struct {
	nodeName    string
	executionID string
}

func NewRestartExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *RestartExecutor {
	return &RestartExecutor{
		Logger:        logger,
		Client:        c,
		DaemonFactory: df,
		executionIDs:  make(map[string][]nodeExecution),
	}
}

func (e *RestartExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	nodes, err := ResolveTargetNodes(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("node-restart: resolve targets: %w", err)
	}

	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("node-restart: %w", err)
	}

	expKey := string(exp.UID)
	for _, n := range nodes {
		endpoint := ResolveDaemonEndpoint(n.Name)
		dc, err := e.DaemonFactory(endpoint)
		if err != nil {
			return fmt.Errorf("node-restart: daemon client for node %s: %w", n.Name, err)
		}

		e.Logger.InfoContext(ctx, "restarting node", "node", n.Name)
		resp, err := dc.ExecNodeChaos(ctx, &daemonv1.NodeChaosRequest{
			ExperimentId: expKey,
			Action:       "restart",
			Parameters: map[string]string{
				"grace_period": params["grace_period"],
			},
		})
		if err != nil {
			return fmt.Errorf("node-restart: exec on node %s: %w", n.Name, err)
		}
		if !resp.Success {
			return fmt.Errorf("node-restart: daemon failure for node %s: %s", n.Name, resp.Message)
		}

		e.mu.Lock()
		e.executionIDs[expKey] = append(e.executionIDs[expKey], nodeExecution{nodeName: n.Name, executionID: resp.ExecutionId})
		e.mu.Unlock()
	}
	return nil
}

func (e *RestartExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	nodes, err := ResolveTargetNodes(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("node-restart: rollback resolve targets: %w", err)
	}

	timeout := 5 * time.Minute
	pollInterval := 10 * time.Second

	for _, n := range nodes {
		e.Logger.InfoContext(ctx, "waiting for node to be ready", "node", n.Name)
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			var current corev1.Node
			if err := e.Client.Get(ctx, client.ObjectKey{Name: n.Name}, &current); err != nil {
				time.Sleep(pollInterval)
				continue
			}
			if isNodeReady(&current) {
				break
			}
			time.Sleep(pollInterval)
		}
	}

	expKey := string(exp.UID)
	e.mu.Lock()
	delete(e.executionIDs, expKey)
	e.mu.Unlock()

	return nil
}

func (e *RestartExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if exp.Spec.Target.LabelSelector == nil && len(exp.Spec.Target.Names) == 0 {
		return fmt.Errorf("node-restart: target must specify names or labelSelector")
	}
	return nil
}

func isNodeReady(node *corev1.Node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
