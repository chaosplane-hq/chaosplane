package extension

import (
	"context"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

// ChaosPlugin defines the interface for custom chaos action plugins.
// It extends the core Executor pattern with plugin metadata, allowing
// third-party actions to be registered and discovered at runtime.
type ChaosPlugin interface {
	// Name returns the unique identifier for this plugin.
	Name() string

	// Version returns the semantic version of this plugin.
	Version() string

	// Execute runs the custom chaos action against the target.
	Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error

	// Rollback reverses the custom chaos action, restoring the target to its original state.
	Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error

	// Validate checks whether the experiment spec is valid for this plugin.
	Validate(exp *v1alpha1.ChaosExperiment) error
}
