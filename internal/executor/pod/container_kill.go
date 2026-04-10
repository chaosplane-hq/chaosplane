package pod

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

var _ executor.Executor = (*ContainerKillExecutor)(nil)

type ContainerKillExecutor struct {
	Logger        *slog.Logger
	Client        client.Client
	DaemonFactory DaemonClientFactory
}

func NewContainerKillExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *ContainerKillExecutor {
	return &ContainerKillExecutor{Logger: logger, Client: c, DaemonFactory: df}
}

func (e *ContainerKillExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	pods, err := ResolveTargetPods(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("container-kill: resolve targets: %w", err)
	}

	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("container-kill: %w", err)
	}

	for _, p := range pods {
		endpoint := ResolveDaemonEndpoint(p.Spec.NodeName)
		dc, err := e.DaemonFactory(endpoint)
		if err != nil {
			return fmt.Errorf("container-kill: daemon client for node %s: %w", p.Spec.NodeName, err)
		}

		e.Logger.InfoContext(ctx, "killing container", "pod", p.Name, "container", params["containerName"], "node", p.Spec.NodeName)
		resp, err := dc.ExecStressChaos(ctx, &daemonv1.StressChaosRequest{
			ExperimentId: string(exp.UID),
			StressorType: "container-kill",
			Parameters: map[string]string{
				"containerName": params["containerName"],
				"podName":       p.Name,
				"podNamespace":  p.Namespace,
			},
		})
		if err != nil {
			return fmt.Errorf("container-kill: exec on pod %s/%s: %w", p.Namespace, p.Name, err)
		}
		if !resp.Success {
			return fmt.Errorf("container-kill: daemon returned failure for %s/%s: %s", p.Namespace, p.Name, resp.Message)
		}
	}
	return nil
}

func (e *ContainerKillExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *ContainerKillExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if exp.Spec.Target.Namespace == "" {
		return fmt.Errorf("container-kill: target namespace is required")
	}
	if exp.Spec.Target.LabelSelector == nil && len(exp.Spec.Target.Names) == 0 {
		return fmt.Errorf("container-kill: target must specify names or labelSelector")
	}
	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("container-kill: %w", err)
	}
	if params["containerName"] == "" {
		return fmt.Errorf("container-kill: containerName parameter is required")
	}
	return nil
}
