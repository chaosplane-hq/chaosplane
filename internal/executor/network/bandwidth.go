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

var _ executor.Executor = (*BandwidthExecutor)(nil)

type BandwidthExecutor struct {
	baseNetworkExecutor
}

func NewBandwidthExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *BandwidthExecutor {
	return &BandwidthExecutor{baseNetworkExecutor: baseNetworkExecutor{Logger: logger, Client: c, DaemonFactory: df}}
}

func (e *BandwidthExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("network-bandwidth: %w", err)
	}
	return e.execNetworkChaos(ctx, exp, "bandwidth", params, "network-bandwidth")
}

func (e *BandwidthExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	return e.rollbackExecutions(ctx, string(exp.UID))
}

func (e *BandwidthExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if err := validateTarget(exp, "network-bandwidth"); err != nil {
		return err
	}
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("network-bandwidth: %w", err)
	}
	if params["rate"] == "" {
		return fmt.Errorf("network-bandwidth: rate parameter is required")
	}
	if params["burst"] == "" {
		return fmt.Errorf("network-bandwidth: burst parameter is required")
	}
	if params["latency"] == "" {
		return fmt.Errorf("network-bandwidth: latency parameter is required")
	}
	return nil
}
