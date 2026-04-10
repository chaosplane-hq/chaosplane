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

var _ executor.Executor = (*PartitionExecutor)(nil)

type PartitionExecutor struct {
	baseNetworkExecutor
}

func NewPartitionExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *PartitionExecutor {
	return &PartitionExecutor{baseNetworkExecutor: baseNetworkExecutor{Logger: logger, Client: c, DaemonFactory: df}}
}

func (e *PartitionExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("network-partition: %w", err)
	}
	return e.execNetworkChaos(ctx, exp, "partition", params, "network-partition")
}

func (e *PartitionExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	return e.rollbackExecutions(ctx, string(exp.UID))
}

func (e *PartitionExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if err := validateTarget(exp, "network-partition"); err != nil {
		return err
	}
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("network-partition: %w", err)
	}
	if params["target_cidr"] == "" {
		return fmt.Errorf("network-partition: target_cidr parameter is required")
	}
	direction := params["direction"]
	if direction == "" {
		return fmt.Errorf("network-partition: direction parameter is required")
	}
	if direction != "ingress" && direction != "egress" && direction != "both" {
		return fmt.Errorf("network-partition: direction must be ingress, egress, or both")
	}
	return nil
}
