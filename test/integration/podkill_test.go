//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

const selfVerifyNamespace = "chaosplane-integration"

// TestHarnessSelfVerify_PodKill is the harness's end-to-end self-check against a
// KNOWN-GOOD fault. It deploys a target pod, runs a pod-kill ChaosExperiment via
// the operator, and asserts the original pod is actually deleted. This proves
// the harness can connect, deploy, drive an experiment, and observe its effect,
// so fault tests (T6-T11, T20) can rely on the same plumbing.
func TestHarnessSelfVerify_PodKill(t *testing.T) {
	h := requireCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	h.EnsureNamespace(ctx, t, selfVerifyNamespace)

	target := h.DeployPod(ctx, t, PodSpec{
		Name:      "podkill-target",
		Namespace: selfVerifyNamespace,
		Labels:    map[string]string{"app": "podkill-target"},
	})
	originalUID := target.UID

	exp := newPodKillExperiment(selfVerifyNamespace, "harness-podkill", "podkill-target")
	created, err := h.Dynamic.Resource(chaosExperimentGVR).
		Namespace(selfVerifyNamespace).
		Create(ctx, exp, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create pod-kill experiment: %v", err)
	}
	t.Cleanup(func() {
		_ = h.Dynamic.Resource(chaosExperimentGVR).
			Namespace(selfVerifyNamespace).
			Delete(context.Background(), created.GetName(), metav1.DeleteOptions{})
	})

	if err := h.waitForPodGone(ctx, selfVerifyNamespace, "podkill-target", originalUID, 3*time.Minute); err != nil {
		phase := experimentPhase(ctx, h, selfVerifyNamespace, created.GetName())
		t.Fatalf("pod-kill did not delete target pod (experiment phase=%q): %v", phase, err)
	}

	t.Log("pod-kill self-verification passed: target pod was deleted by the experiment")
}

// newPodKillExperiment builds a minimal pod-kill ChaosExperiment targeting pods
// matched by app=<targetApp>. Built unstructured so the harness needs no compile
// dependency on the operator's typed client.
func newPodKillExperiment(namespace, name, targetApp string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "chaos.chaosplane.dev/v1alpha1",
			"kind":       "ChaosExperiment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels":    map[string]interface{}{"test": "integration"},
			},
			"spec": map[string]interface{}{
				"target": map[string]interface{}{
					"kind":      "Pod",
					"namespace": namespace,
					"labelSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{"app": targetApp},
					},
				},
				"action": map[string]interface{}{
					"type": "pod-kill",
					"parameters": map[string]interface{}{
						"gracePeriodSeconds": "0",
					},
				},
				"duration": "30s",
			},
		},
	}
}

// waitForPodGone blocks until the pod with the given original UID is no longer
// present (deleted), or a replacement with a different UID appears. Either
// outcome confirms pod-kill acted on the target.
func (h *Harness) waitForPodGone(ctx context.Context, namespace, name string, originalUID types.UID, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := h.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, nil
		}
		if pod.UID != originalUID {
			return true, nil
		}
		if pod.DeletionTimestamp != nil {
			return true, nil
		}
		return false, nil
	})
}

func experimentPhase(ctx context.Context, h *Harness, namespace, name string) string {
	obj, err := h.Dynamic.Resource(chaosExperimentGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "<unknown>"
	}
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	return phase
}
