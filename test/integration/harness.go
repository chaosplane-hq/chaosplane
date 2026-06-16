//go:build integration

// Package integration provides a kind-backed harness for asserting the real
// effects of chaos faults: network degradation (RTT, loss, DNS, HTTP), cgroup
// CPU/memory pressure, and pod lifecycle. Fault tests (T6-T11, T20) compose the
// exported helpers here rather than re-implementing measurement logic.
package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/exec"
)

// chaosExperimentGVR mirrors the GVR used by the e2e harness so fixtures and
// experiments resolve identically across suites.
var chaosExperimentGVR = schema.GroupVersionResource{
	Group:    "chaos.chaosplane.dev",
	Version:  "v1alpha1",
	Resource: "chaosexperiments",
}

// Harness bundles the clients and config needed to drive a live cluster and
// exec into workloads for measurement.
type Harness struct {
	Clientset kubernetes.Interface
	Dynamic   dynamic.Interface
	Config    *rest.Config
}

// requireCluster skips the test unless INTEGRATION=1, then builds a Harness from
// KUBECONFIG (defaulting to ~/.kube/config). Heavy cluster work stays gated so
// `go test ./...` without kind still passes.
func requireCluster(t *testing.T) *Harness {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("integration tests require a live cluster; set INTEGRATION=1 to run")
	}

	h, err := newHarness()
	if err != nil {
		t.Fatalf("connect to cluster: %v", err)
	}
	return h
}

func newHarness() (*Harness, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return &Harness{Clientset: clientset, Dynamic: dynClient, Config: config}, nil
}

// ExecResult captures the streams and exit status of a command run inside a pod.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Exec runs cmd in the first container of pod and returns its streams. A
// non-zero exit code is reported via ExecResult.ExitCode (not an error) so
// callers can assert on expected command failures (e.g. ping with 100% loss).
func (h *Harness) Exec(ctx context.Context, namespace, pod string, cmd ...string) (ExecResult, error) {
	req := h.Clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: cmd,
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(h.Config, "POST", req.URL())
	if err != nil {
		return ExecResult{}, fmt.Errorf("init executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	res := ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		var codeErr exec.CodeExitError
		if errors.As(err, &codeErr) {
			res.ExitCode = codeErr.Code
			return res, nil
		}
		return res, fmt.Errorf("exec %v in %s/%s: %w", cmd, namespace, pod, err)
	}
	return res, nil
}

// EnsureNamespace creates namespace if absent and registers a cleanup that
// deletes it when the test finishes.
func (h *Harness) EnsureNamespace(ctx context.Context, t *testing.T, namespace string) {
	t.Helper()
	_, err := h.Clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace %s: %v", namespace, err)
	}
	t.Cleanup(func() {
		_ = h.Clientset.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	})
}

// PodSpec describes a workload the harness deploys for measurement. Image
// defaults to a busybox-style image that ships ping, nslookup, and wget.
type PodSpec struct {
	Name      string
	Namespace string
	Image     string
	Labels    map[string]string
	// Command overrides the container entrypoint; defaults to a long sleep so
	// the pod stays alive for exec-based probing.
	Command []string
}

const defaultProbeImage = "busybox:1.36"

// DeployPod creates a pod, waits for it to become Ready, and registers cleanup.
// It returns the running pod. Used for both target workloads and probe pods.
func (h *Harness) DeployPod(ctx context.Context, t *testing.T, spec PodSpec) *corev1.Pod {
	t.Helper()

	image := spec.Image
	if image == "" {
		image = defaultProbeImage
	}
	command := spec.Command
	if command == nil {
		command = []string{"sleep", "infinity"}
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
			Labels:    spec.Labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    "main",
				Image:   image,
				Command: command,
			}},
		},
	}

	created, err := h.Clientset.CoreV1().Pods(spec.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create pod %s/%s: %v", spec.Namespace, spec.Name, err)
	}
	t.Cleanup(func() {
		_ = h.Clientset.CoreV1().Pods(spec.Namespace).Delete(
			context.Background(), spec.Name,
			metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(0)},
		)
	})

	if err := h.WaitForPodReady(ctx, spec.Namespace, spec.Name, 2*time.Minute); err != nil {
		t.Fatalf("pod %s/%s not ready: %v", spec.Namespace, spec.Name, err)
	}
	return created
}

// WaitForPodReady blocks until the pod reports the Ready condition or timeout.
func (h *Harness) WaitForPodReady(ctx context.Context, namespace, name string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := h.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}

// PodIP returns the cluster IP assigned to a pod, useful as a network target.
func (h *Harness) PodIP(ctx context.Context, namespace, name string) (string, error) {
	pod, err := h.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get pod %s/%s: %w", namespace, name, err)
	}
	if pod.Status.PodIP == "" {
		return "", fmt.Errorf("pod %s/%s has no IP yet", namespace, name)
	}
	return pod.Status.PodIP, nil
}

func ptrInt64(v int64) *int64 { return &v }

// trimmed collapses surrounding whitespace from command output.
func trimmed(s string) string { return strings.TrimSpace(s) }
