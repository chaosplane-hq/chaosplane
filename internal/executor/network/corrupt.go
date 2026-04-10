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

var _ executor.Executor = (*CorruptExecutor)(nil)

type CorruptExecutor struct {
	baseNetworkExecutor
}

func NewCorruptExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *CorruptExecutor {
	return &CorruptExecutor{baseNetworkExecutor: baseNetworkExecutor{Logger: logger, Client: c, DaemonFactory: df}}
}

func (e *CorruptExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("network-corrupt: %w", err)
	}
	return e.execNetworkChaos(ctx, exp, "corrupt", params, "network-corrupt")
}

func (e *CorruptExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	return e.rollbackExecutions(ctx, string(exp.UID))
}

func (e *CorruptExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if err := validateTarget(exp, "network-corrupt"); err != nil {
		return err
	}
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("network-corrupt: %w", err)
	}
	if params["percent"] == "" {
		return fmt.Errorf("network-corrupt: percent parameter is required")
	}
	return nil
}
