package extension

import (
	"context"
	"fmt"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

// ExampleLatencyPlugin injects artificial latency into network traffic.
// It demonstrates how to implement the ChaosPlugin interface.
type ExampleLatencyPlugin struct{}

func (p *ExampleLatencyPlugin) Name() string {
	return "example-latency"
}

func (p *ExampleLatencyPlugin) Version() string {
	return "0.1.0"
}

func (p *ExampleLatencyPlugin) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	fmt.Printf("[%s] executing latency injection for experiment %s/%s\n",
		p.Name(), exp.Namespace, exp.Name)
	return nil
}

func (p *ExampleLatencyPlugin) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	fmt.Printf("[%s] rolling back latency injection for experiment %s/%s\n",
		p.Name(), exp.Namespace, exp.Name)
	return nil
}

func (p *ExampleLatencyPlugin) Validate(exp *v1alpha1.ChaosExperiment) error {
	if exp.Spec.Action.Type == "" {
		return fmt.Errorf("action type is required")
	}
	return nil
}
