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

var _ executor.Executor = (*EBPFDelayExecutor)(nil)

type EBPFDelayExecutor struct {
	baseNetworkExecutor
}

func NewEBPFDelayExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *EBPFDelayExecutor {
	return &EBPFDelayExecutor{baseNetworkExecutor: baseNetworkExecutor{Logger: logger, Client: c, DaemonFactory: df}}
}

func (e *EBPFDelayExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("ebpf-network-delay: %w", err)
	}
	params["mode"] = "ebpf"
	return e.execNetworkChaos(ctx, exp, "delay", params, "ebpf-network-delay")
}

func (e *EBPFDelayExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	return e.rollbackExecutions(ctx, string(exp.UID))
}

func (e *EBPFDelayExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if err := validateTarget(exp, "ebpf-network-delay"); err != nil {
		return err
	}
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("ebpf-network-delay: %w", err)
	}
	if params["latency"] == "" {
		return fmt.Errorf("ebpf-network-delay: latency parameter is required")
	}
	return nil
}
