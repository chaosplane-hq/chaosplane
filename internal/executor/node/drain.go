package node

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

var _ executor.Executor = (*DrainExecutor)(nil)

type DrainExecutor struct {
	Logger    *slog.Logger
	Client    client.Client
	Clientset kubernetes.Interface
}

func NewDrainExecutor(logger *slog.Logger, c client.Client, cs kubernetes.Interface) *DrainExecutor {
	return &DrainExecutor{Logger: logger, Client: c, Clientset: cs}
}

func (e *DrainExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	nodes, err := ResolveTargetNodes(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("node-drain: resolve targets: %w", err)
	}

	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("node-drain: %w", err)
	}

	timeout := 300 * time.Second
	if v, ok := params["timeout"]; ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("node-drain: invalid timeout %q: %w", v, err)
		}
		timeout = d
	}

	ignoreDaemonSets := true
	if v, ok := params["ignoreDaemonSets"]; ok {
		ignoreDaemonSets, _ = strconv.ParseBool(v)
	}

	deleteEmptyDirData := true
	if v, ok := params["deleteEmptyDirData"]; ok {
		deleteEmptyDirData, _ = strconv.ParseBool(v)
	}

	for _, n := range nodes {
		e.Logger.InfoContext(ctx, "cordoning node", "node", n.Name)
		n.Spec.Unschedulable = true
		if err := e.Client.Update(ctx, &n); err != nil {
			return fmt.Errorf("node-drain: cordon node %s: %w", n.Name, err)
		}

		if err := e.evictPods(ctx, n.Name, timeout, ignoreDaemonSets, deleteEmptyDirData); err != nil {
			return fmt.Errorf("node-drain: evict pods from node %s: %w", n.Name, err)
		}
	}
	return nil
}

func (e *DrainExecutor) evictPods(ctx context.Context, nodeName string, timeout time.Duration, ignoreDaemonSets, deleteEmptyDirData bool) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pods, err := e.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		return fmt.Errorf("list pods on node %s: %w", nodeName, err)
	}

	for _, p := range pods.Items {
		// Skip mirror pods (static pods)
		if _, ok := p.Annotations["kubernetes.io/config.mirror"]; ok {
			continue
		}

		// Skip DaemonSet pods if configured
		if ignoreDaemonSets {
			isDaemonSet := false
			for _, ref := range p.OwnerReferences {
				if ref.Kind == "DaemonSet" {
					isDaemonSet = true
					break
				}
			}
			if isDaemonSet {
				continue
			}
		}

		// Skip pods using emptyDir if not configured to delete
		if !deleteEmptyDirData {
			hasEmptyDir := false
			for _, v := range p.Spec.Volumes {
				if v.EmptyDir != nil {
					hasEmptyDir = true
					break
				}
			}
			if hasEmptyDir {
				continue
			}
		}

		e.Logger.InfoContext(ctx, "evicting pod", "pod", p.Name, "namespace", p.Namespace)
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      p.Name,
				Namespace: p.Namespace,
			},
		}
		if err := e.Clientset.PolicyV1().Evictions(p.Namespace).Evict(ctx, eviction); err != nil {
			return fmt.Errorf("evict pod %s/%s: %w", p.Namespace, p.Name, err)
		}
	}
	return nil
}

func (e *DrainExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	nodes, err := ResolveTargetNodes(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("node-drain: rollback resolve targets: %w", err)
	}

	for _, n := range nodes {
		e.Logger.InfoContext(ctx, "uncordoning node", "node", n.Name)
		n.Spec.Unschedulable = false
		if err := e.Client.Update(ctx, &n); err != nil {
			return fmt.Errorf("node-drain: uncordon node %s: %w", n.Name, err)
		}
	}
	return nil
}

func (e *DrainExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if exp.Spec.Target.LabelSelector == nil && len(exp.Spec.Target.Names) == 0 {
		return fmt.Errorf("node-drain: target must specify names or labelSelector")
	}
	return nil
}
