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

var _ executor.Executor = (*EBPFDNSExecutor)(nil)

type EBPFDNSExecutor struct {
	baseNetworkExecutor
}

func NewEBPFDNSExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *EBPFDNSExecutor {
	return &EBPFDNSExecutor{baseNetworkExecutor: baseNetworkExecutor{Logger: logger, Client: c, DaemonFactory: df}}
}

func (e *EBPFDNSExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("ebpf-dns-chaos: %w", err)
	}
	params["mode"] = "ebpf"
	return e.execNetworkChaos(ctx, exp, "dns-error", params, "ebpf-dns-chaos")
}

func (e *EBPFDNSExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	return e.rollbackExecutions(ctx, string(exp.UID))
}

func (e *EBPFDNSExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if err := validateTarget(exp, "ebpf-dns-chaos"); err != nil {
		return err
	}
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("ebpf-dns-chaos: %w", err)
	}
	if params["domains"] == "" {
		return fmt.Errorf("ebpf-dns-chaos: domains parameter is required")
	}
	return nil
}
