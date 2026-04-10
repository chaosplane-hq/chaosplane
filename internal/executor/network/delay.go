package network

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

var _ executor.Executor = (*DelayExecutor)(nil)

type DelayExecutor struct {
	baseNetworkExecutor
}

func NewDelayExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *DelayExecutor {
	return &DelayExecutor{baseNetworkExecutor: baseNetworkExecutor{Logger: logger, Client: c, DaemonFactory: df}}
}

func (e *DelayExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("network-delay: %w", err)
	}
	return e.execNetworkChaos(ctx, exp, "delay", params, "network-delay")
}

func (e *DelayExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	return e.rollbackExecutions(ctx, string(exp.UID))
}

func (e *DelayExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if err := validateTarget(exp, "network-delay"); err != nil {
		return err
	}
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("network-delay: %w", err)
	}
	if params["latency"] == "" {
		return fmt.Errorf("network-delay: latency parameter is required")
	}
	return nil
}
