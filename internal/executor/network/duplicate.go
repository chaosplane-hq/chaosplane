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

var _ executor.Executor = (*DuplicateExecutor)(nil)

type DuplicateExecutor struct {
	baseNetworkExecutor
}

func NewDuplicateExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *DuplicateExecutor {
	return &DuplicateExecutor{baseNetworkExecutor: baseNetworkExecutor{Logger: logger, Client: c, DaemonFactory: df}}
}

func (e *DuplicateExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("network-duplicate: %w", err)
	}
	return e.execNetworkChaos(ctx, exp, "duplicate", params, "network-duplicate")
}

func (e *DuplicateExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	return e.rollbackExecutions(ctx, string(exp.UID))
}

func (e *DuplicateExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if err := validateTarget(exp, "network-duplicate"); err != nil {
		return err
	}
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("network-duplicate: %w", err)
	}
	if params["percent"] == "" {
		return fmt.Errorf("network-duplicate: percent parameter is required")
	}
	return nil
}
