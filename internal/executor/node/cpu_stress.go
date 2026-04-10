package node

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

var _ executor.Executor = (*CPUStressExecutor)(nil)

type CPUStressExecutor struct {
	Logger        *slog.Logger
	Client        client.Client
	DaemonFactory DaemonClientFactory

	mu           sync.Mutex
	executionIDs map[string][]nodeExecution
}

func NewCPUStressExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *CPUStressExecutor {
	return &CPUStressExecutor{
		Logger:        logger,
		Client:        c,
		DaemonFactory: df,
		executionIDs:  make(map[string][]nodeExecution),
	}
}

func (e *CPUStressExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	nodes, err := ResolveTargetNodes(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("node-cpu-stress: resolve targets: %w", err)
	}

	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("node-cpu-stress: %w", err)
	}

	expKey := string(exp.UID)
	for _, n := range nodes {
		endpoint := ResolveDaemonEndpoint(n.Name)
		dc, err := e.DaemonFactory(endpoint)
		if err != nil {
			return fmt.Errorf("node-cpu-stress: daemon client for node %s: %w", n.Name, err)
		}

		e.Logger.InfoContext(ctx, "applying node cpu stress", "node", n.Name)
		resp, err := dc.ExecStressChaos(ctx, &daemonv1.StressChaosRequest{
			ExperimentId: expKey,
			StressorType: "cpu",
			Parameters: map[string]string{
				"workers":  params["workers"],
				"load":     params["load"],
				"duration": params["duration"],
			},
		})
		if err != nil {
			return fmt.Errorf("node-cpu-stress: exec on node %s: %w", n.Name, err)
		}
		if !resp.Success {
			return fmt.Errorf("node-cpu-stress: daemon failure for node %s: %s", n.Name, resp.Message)
		}

		e.mu.Lock()
		e.executionIDs[expKey] = append(e.executionIDs[expKey], nodeExecution{nodeName: n.Name, executionID: resp.ExecutionId})
		e.mu.Unlock()
	}
	return nil
}

func (e *CPUStressExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	expKey := string(exp.UID)
	e.mu.Lock()
	execs := e.executionIDs[expKey]
	delete(e.executionIDs, expKey)
	e.mu.Unlock()

	for _, ne := range execs {
		endpoint := ResolveDaemonEndpoint(ne.nodeName)
		dc, err := e.DaemonFactory(endpoint)
		if err != nil {
			return fmt.Errorf("node-cpu-stress: rollback daemon client for node %s: %w", ne.nodeName, err)
		}
		if _, err := dc.CancelChaos(ctx, &daemonv1.CancelRequest{ExecutionId: ne.executionID}); err != nil {
			return fmt.Errorf("node-cpu-stress: cancel execution %s: %w", ne.executionID, err)
		}
	}
	return nil
}

func (e *CPUStressExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if exp.Spec.Target.LabelSelector == nil && len(exp.Spec.Target.Names) == 0 {
		return fmt.Errorf("node-cpu-stress: target must specify names or labelSelector")
	}
	return nil
}
