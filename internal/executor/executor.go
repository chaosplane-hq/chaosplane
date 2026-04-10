package executor

import (
	"context"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

// Executor defines the interface for chaos experiment actions.
type Executor interface {
	// Execute runs the chaos action against the target.
	Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error

	// Rollback reverses the chaos action, restoring the target to its original state.
	Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error

	// Validate checks whether the experiment spec is valid for this executor.
	Validate(exp *v1alpha1.ChaosExperiment) error
}
