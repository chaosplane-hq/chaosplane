package executor_test

import (
	"context"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

type mockExecutor struct{}

func (m *mockExecutor) Execute(_ context.Context, _ *v1alpha1.ChaosExperiment) error  { return nil }
func (m *mockExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error { return nil }
func (m *mockExecutor) Validate(_ *v1alpha1.ChaosExperiment) error                    { return nil }
