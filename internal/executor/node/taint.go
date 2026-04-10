package node

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

var _ executor.Executor = (*TaintExecutor)(nil)

type TaintExecutor struct {
	Logger *slog.Logger
	Client client.Client
}

func NewTaintExecutor(logger *slog.Logger, c client.Client) *TaintExecutor {
	return &TaintExecutor{Logger: logger, Client: c}
}

func (e *TaintExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	nodes, err := ResolveTargetNodes(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("node-taint: resolve targets: %w", err)
	}

	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("node-taint: %w", err)
	}

	taint, err := buildTaint(params)
	if err != nil {
		return fmt.Errorf("node-taint: %w", err)
	}

	for _, n := range nodes {
		e.Logger.InfoContext(ctx, "adding taint to node", "node", n.Name, "key", taint.Key, "effect", taint.Effect)

		exists := false
		for _, t := range n.Spec.Taints {
			if t.Key == taint.Key && t.Effect == taint.Effect {
				exists = true
				break
			}
		}
		if exists {
			continue
		}

		n.Spec.Taints = append(n.Spec.Taints, taint)
		if err := e.Client.Update(ctx, &n); err != nil {
			return fmt.Errorf("node-taint: add taint to node %s: %w", n.Name, err)
		}
	}
	return nil
}

func (e *TaintExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	nodes, err := ResolveTargetNodes(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("node-taint: rollback resolve targets: %w", err)
	}

	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("node-taint: rollback parse params: %w", err)
	}

	taint, err := buildTaint(params)
	if err != nil {
		return fmt.Errorf("node-taint: rollback: %w", err)
	}

	for _, n := range nodes {
		e.Logger.InfoContext(ctx, "removing taint from node", "node", n.Name, "key", taint.Key)

		filtered := make([]corev1.Taint, 0, len(n.Spec.Taints))
		for _, t := range n.Spec.Taints {
			if t.Key == taint.Key && t.Effect == taint.Effect {
				continue
			}
			filtered = append(filtered, t)
		}
		n.Spec.Taints = filtered
		if err := e.Client.Update(ctx, &n); err != nil {
			return fmt.Errorf("node-taint: remove taint from node %s: %w", n.Name, err)
		}
	}
	return nil
}

func (e *TaintExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if exp.Spec.Target.LabelSelector == nil && len(exp.Spec.Target.Names) == 0 {
		return fmt.Errorf("node-taint: target must specify names or labelSelector")
	}

	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("node-taint: %w", err)
	}

	if params["key"] == "" {
		return fmt.Errorf("node-taint: parameter 'key' is required")
	}

	effect := params["effect"]
	switch corev1.TaintEffect(effect) {
	case corev1.TaintEffectNoSchedule, corev1.TaintEffectNoExecute, corev1.TaintEffectPreferNoSchedule:
	default:
		return fmt.Errorf("node-taint: invalid effect %q, must be NoSchedule, NoExecute, or PreferNoSchedule", effect)
	}

	return nil
}

func buildTaint(params map[string]string) (corev1.Taint, error) {
	key := params["key"]
	if key == "" {
		return corev1.Taint{}, fmt.Errorf("parameter 'key' is required")
	}

	effect := corev1.TaintEffect(params["effect"])
	switch effect {
	case corev1.TaintEffectNoSchedule, corev1.TaintEffectNoExecute, corev1.TaintEffectPreferNoSchedule:
	default:
		return corev1.Taint{}, fmt.Errorf("invalid effect %q", params["effect"])
	}

	return corev1.Taint{
		Key:    key,
		Value:  params["value"],
		Effect: effect,
	}, nil
}
