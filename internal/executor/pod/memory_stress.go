package pod

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

var _ executor.Executor = (*MemoryStressExecutor)(nil)

type MemoryStressExecutor struct {
	Logger        *slog.Logger
	Client        client.Client
	DaemonFactory DaemonClientFactory

	mu           sync.Mutex
	executionIDs map[string][]nodeExecution
}

func NewMemoryStressExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *MemoryStressExecutor {
	return &MemoryStressExecutor{
		Logger:        logger,
		Client:        c,
		DaemonFactory: df,
		executionIDs:  make(map[string][]nodeExecution),
	}
}

func (e *MemoryStressExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	pods, err := ResolveTargetPods(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("pod-memory-stress: resolve targets: %w", err)
	}

	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("pod-memory-stress: %w", err)
	}

	expKey := string(exp.UID)
	for _, p := range pods {
		endpoint := ResolveDaemonEndpoint(p.Spec.NodeName)
		dc, err := e.DaemonFactory(endpoint)
		if err != nil {
			return fmt.Errorf("pod-memory-stress: daemon client for node %s: %w", p.Spec.NodeName, err)
		}

		e.Logger.InfoContext(ctx, "applying memory stress", "pod", p.Name, "node", p.Spec.NodeName)
		resp, err := dc.ExecStressChaos(ctx, &daemonv1.StressChaosRequest{
			ExperimentId: expKey,
			StressorType: "memory",
			Parameters: map[string]string{
				"workers":      params["workers"],
				"size":         params["size"],
				"duration":     params["duration"],
				"podName":      p.Name,
				"podNamespace": p.Namespace,
				"containerId":  ContainerID(&p),
				"nodeName":     p.Spec.NodeName,
			},
		})
		if err != nil {
			return fmt.Errorf("pod-memory-stress: exec on pod %s/%s: %w", p.Namespace, p.Name, err)
		}
		if !resp.Success {
			return fmt.Errorf("pod-memory-stress: daemon failure for %s/%s: %s", p.Namespace, p.Name, resp.Message)
		}

		e.mu.Lock()
		e.executionIDs[expKey] = append(e.executionIDs[expKey], nodeExecution{nodeName: p.Spec.NodeName, executionID: resp.ExecutionId})
		e.mu.Unlock()
	}
	return nil
}

func (e *MemoryStressExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	expKey := string(exp.UID)
	e.mu.Lock()
	execs := e.executionIDs[expKey]
	delete(e.executionIDs, expKey)
	e.mu.Unlock()

	for _, ne := range execs {
		endpoint := ResolveDaemonEndpoint(ne.nodeName)
		dc, err := e.DaemonFactory(endpoint)
		if err != nil {
			return fmt.Errorf("pod-memory-stress: rollback daemon client for node %s: %w", ne.nodeName, err)
		}
		if _, err := dc.CancelChaos(ctx, &daemonv1.CancelRequest{ExecutionId: ne.executionID}); err != nil {
			return fmt.Errorf("pod-memory-stress: cancel execution %s: %w", ne.executionID, err)
		}
	}
	return nil
}

func (e *MemoryStressExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if exp.Spec.Target.Namespace == "" {
		return fmt.Errorf("pod-memory-stress: target namespace is required")
	}
	if exp.Spec.Target.LabelSelector == nil && len(exp.Spec.Target.Names) == 0 {
		return fmt.Errorf("pod-memory-stress: target must specify names or labelSelector")
	}
	return nil
}
