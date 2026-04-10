package probe

import (
	"context"
	"fmt"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Probe interface {
	Run(ctx context.Context) (bool, error)
}

func NewProbe(spec v1alpha1.ProbeSpec, k8sClient client.Client) (Probe, error) {
	switch spec.Type {
	case v1alpha1.ProbeTypePrometheus:
		if spec.Prometheus == nil {
			return nil, fmt.Errorf("probe %q: prometheus config required", spec.Name)
		}
		return &prometheusProbe{spec: *spec.Prometheus}, nil
	case v1alpha1.ProbeTypeHTTP:
		if spec.HTTP == nil {
			return nil, fmt.Errorf("probe %q: http config required", spec.Name)
		}
		return &httpProbe{spec: *spec.HTTP}, nil
	case v1alpha1.ProbeTypeK8s:
		if spec.K8s == nil {
			return nil, fmt.Errorf("probe %q: k8s config required", spec.Name)
		}
		return &k8sProbe{spec: *spec.K8s, client: k8sClient}, nil
	default:
		return nil, fmt.Errorf("probe %q: unknown type %q", spec.Name, spec.Type)
	}
}

func RunAll(ctx context.Context, probes []v1alpha1.ProbeSpec, k8sClient client.Client) (bool, error) {
	for _, spec := range probes {
		p, err := NewProbe(spec, k8sClient)
		if err != nil {
			return false, fmt.Errorf("probe %q: %w", spec.Name, err)
		}
		ok, err := p.Run(ctx)
		if err != nil {
			return false, fmt.Errorf("probe %q: %w", spec.Name, err)
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
