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

var _ executor.Executor = (*EBPFLossExecutor)(nil)

type EBPFLossExecutor struct {
	baseNetworkExecutor
}

func NewEBPFLossExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *EBPFLossExecutor {
	return &EBPFLossExecutor{baseNetworkExecutor: baseNetworkExecutor{Logger: logger, Client: c, DaemonFactory: df}}
}

func (e *EBPFLossExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("ebpf-network-loss: %w", err)
	}
	params["mode"] = "ebpf"
	return e.execNetworkChaos(ctx, exp, "loss", params, "ebpf-network-loss")
}

func (e *EBPFLossExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	return e.rollbackExecutions(ctx, string(exp.UID))
}

func (e *EBPFLossExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if err := validateTarget(exp, "ebpf-network-loss"); err != nil {
		return err
	}
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("ebpf-network-loss: %w", err)
	}
	if params["percent"] == "" {
		return fmt.Errorf("ebpf-network-loss: percent parameter is required")
	}
	return nil
}
