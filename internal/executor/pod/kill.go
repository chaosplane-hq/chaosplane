package pod

import (
	"context"
	"log/slog"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

type KillExecutor struct {
	Logger *slog.Logger
}

var _ executor.Executor = (*KillExecutor)(nil)

func NewKillExecutor(logger *slog.Logger) *KillExecutor {
	return &KillExecutor{Logger: logger}
}

func (e *KillExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	e.Logger.InfoContext(ctx, "pod-kill execute (stub)",
		"experiment", exp.Name,
		"namespace", exp.Namespace,
	)
	return nil
}

func (e *KillExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	e.Logger.InfoContext(ctx, "pod-kill rollback (stub)",
		"experiment", exp.Name,
		"namespace", exp.Namespace,
	)
	return nil
}

func (e *KillExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	e.Logger.Info("pod-kill validate (stub)",
		"experiment", exp.Name,
		"namespace", exp.Namespace,
	)
	return nil
}
