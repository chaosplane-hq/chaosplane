package probe

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

type k8sProbe struct {
	spec   v1alpha1.K8sProbe
	client client.Client
}

func (p *k8sProbe) Run(ctx context.Context) (bool, error) {
	switch p.spec.Resource {
	case "Pod", "pods":
		return p.checkPods(ctx)
	default:
		return false, fmt.Errorf("unsupported resource kind %q", p.spec.Resource)
	}
}

func (p *k8sProbe) checkPods(ctx context.Context) (bool, error) {
	var podList corev1.PodList
	opts := []client.ListOption{}

	if p.spec.Namespace != "" {
		opts = append(opts, client.InNamespace(p.spec.Namespace))
	}
	if p.spec.LabelSelector != "" {
		labels, err := parseSelector(p.spec.LabelSelector)
		if err != nil {
			return false, fmt.Errorf("parsing label selector: %w", err)
		}
		opts = append(opts, client.MatchingLabels(labels))
	}
	if p.spec.FieldSelector != "" {
		opts = append(opts, client.MatchingFields(parseFieldSelector(p.spec.FieldSelector)))
	}

	if err := p.client.List(ctx, &podList, opts...); err != nil {
		return false, fmt.Errorf("listing pods: %w", err)
	}

	readyCount := 0
	for i := range podList.Items {
		if podList.Items[i].Status.Phase == corev1.PodRunning {
			readyCount++
		}
	}

	return readyCount >= p.spec.Condition.MinReady, nil
}

func parseSelector(s string) (map[string]string, error) {
	result := make(map[string]string)
	if s == "" {
		return result, nil
	}
	pairs := splitComma(s)
	for _, pair := range pairs {
		k, v, ok := splitEquals(pair)
		if !ok {
			return nil, fmt.Errorf("invalid selector pair: %q", pair)
		}
		result[k] = v
	}
	return result, nil
}

func parseFieldSelector(s string) map[string]string {
	result := make(map[string]string)
	if s == "" {
		return result
	}
	pairs := splitComma(s)
	for _, pair := range pairs {
		k, v, ok := splitEquals(pair)
		if ok {
			result[k] = v
		}
	}
	return result
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func splitEquals(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}
