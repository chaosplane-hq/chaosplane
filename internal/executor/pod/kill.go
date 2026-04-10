package pod

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

var _ executor.Executor = (*KillExecutor)(nil)

type KillExecutor struct {
	Logger    *slog.Logger
	Client    client.Client
	Clientset kubernetes.Interface
}

func NewKillExecutor(logger *slog.Logger, c client.Client, cs kubernetes.Interface) *KillExecutor {
	return &KillExecutor{Logger: logger, Client: c, Clientset: cs}
}

func (e *KillExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	pods, err := ResolveTargetPods(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("pod-kill: resolve targets: %w", err)
	}

	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("pod-kill: %w", err)
	}

	var gracePeriod int64
	if v, ok := params["gracePeriodSeconds"]; ok {
		gracePeriod, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("pod-kill: invalid gracePeriodSeconds %q: %w", v, err)
		}
	}

	for _, p := range pods {
		e.Logger.InfoContext(ctx, "deleting pod", "pod", p.Name, "namespace", p.Namespace)
		if err := e.Clientset.CoreV1().Pods(p.Namespace).Delete(ctx, p.Name, metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
		}); err != nil {
			return fmt.Errorf("pod-kill: delete pod %s/%s: %w", p.Namespace, p.Name, err)
		}
	}
	return nil
}

func (e *KillExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *KillExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if exp.Spec.Target.Namespace == "" {
		return fmt.Errorf("pod-kill: target namespace is required")
	}
	if exp.Spec.Target.LabelSelector == nil && len(exp.Spec.Target.Names) == 0 {
		return fmt.Errorf("pod-kill: target must specify names or labelSelector")
	}
	return nil
}
