//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	defaultTimeout = 120 * time.Second
	terminalPhases = "Completed,Failed,Aborted"
)

var allExecutors = []struct {
	name    string
	fixture string
}{
	{"pod-kill", "pod-kill.yaml"},
	{"container-kill", "container-kill.yaml"},
	{"pod-cpu-stress", "pod-cpu-stress.yaml"},
	{"pod-memory-stress", "pod-memory-stress.yaml"},
	{"pod-io-stress", "pod-io-stress.yaml"},
	{"pod-dns-error", "pod-dns-error.yaml"},
	{"pod-http-abort", "pod-http-abort.yaml"},
	{"pod-http-delay", "pod-http-delay.yaml"},
	{"network-delay", "network-delay.yaml"},
	{"network-loss", "network-loss.yaml"},
	{"network-corrupt", "network-corrupt.yaml"},
	{"network-duplicate", "network-duplicate.yaml"},
	{"network-partition", "network-partition.yaml"},
	{"network-bandwidth", "network-bandwidth.yaml"},
	{"node-drain", "node-drain.yaml"},
	{"node-taint", "node-taint.yaml"},
	{"node-restart", "node-restart.yaml"},
	{"node-cpu-stress", "node-cpu-stress.yaml"},
	{"stress-cpu", "stress-cpu.yaml"},
	{"stress-memory", "stress-memory.yaml"},
	{"configmap-corrupt", "configmap-corrupt.yaml"},
	{"pvc-fill", "pvc-fill.yaml"},
	{"etcd-latency", "etcd-latency.yaml"},
	{"time-skew", "time-skew.yaml"},
}

var awsExecutors = []struct {
	name    string
	fixture string
}{
	{"aws-ec2-stop", "aws-ec2-stop.yaml"},
	{"aws-rds-failover", "aws-rds-failover.yaml"},
	{"aws-ecs-stop-task", "aws-ecs-stop-task.yaml"},
}

func TestAllExecutors(t *testing.T) {
	c, err := newE2EClient()
	if err != nil {
		t.Fatalf("create e2e client: %v", err)
	}

	for _, tc := range allExecutors {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			defer cancel()

			fixturePath := filepath.Join(fixtureDir(), tc.fixture)
			obj, err := applyFixture(ctx, c, fixturePath)
			if err != nil {
				t.Fatalf("apply fixture %s: %v", tc.fixture, err)
			}

			name := obj.GetName()
			ns := obj.GetNamespace()
			defer func() {
				if cleanErr := cleanupExperiment(context.Background(), c, ns, name); cleanErr != nil {
					t.Logf("cleanup %s/%s: %v", ns, name, cleanErr)
				}
			}()

			phase, err := waitForPhase(ctx, c, ns, name, []string{"Running", "Completed", "Failed"}, defaultTimeout)
			if err != nil {
				t.Fatalf("wait for phase: %v", err)
			}

			t.Logf("experiment %s reached phase %s", name, phase)

			finalObj, err := c.dynamic.Resource(chaosExperimentGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("get final status: %v", err)
			}

			statusPhase, _, _ := unstructured.NestedString(finalObj.Object, "status", "phase")
			if statusPhase == "" {
				t.Error("experiment has no status.phase set")
			}
		})
	}
}

func TestAWSExecutors(t *testing.T) {
	if os.Getenv("AWS_E2E") != "1" {
		t.Skip("skipping AWS e2e tests: set AWS_E2E=1 to enable")
	}

	c, err := newE2EClient()
	if err != nil {
		t.Fatalf("create e2e client: %v", err)
	}

	for _, tc := range awsExecutors {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			defer cancel()

			fixturePath := filepath.Join(fixtureDir(), tc.fixture)
			obj, err := applyFixture(ctx, c, fixturePath)
			if err != nil {
				t.Fatalf("apply fixture %s: %v", tc.fixture, err)
			}

			name := obj.GetName()
			ns := obj.GetNamespace()
			defer func() {
				if cleanErr := cleanupExperiment(context.Background(), c, ns, name); cleanErr != nil {
					t.Logf("cleanup %s/%s: %v", ns, name, cleanErr)
				}
			}()

			phase, err := waitForPhase(ctx, c, ns, name, []string{"Running", "Completed", "Failed"}, defaultTimeout)
			if err != nil {
				t.Fatalf("wait for phase: %v", err)
			}

			t.Logf("experiment %s reached phase %s", name, phase)
		})
	}
}

func TestClusterExecutors(t *testing.T) {
	c, err := newE2EClient()
	if err != nil {
		t.Fatalf("create e2e client: %v", err)
	}

	clusterFixtures := []struct {
		name    string
		fixture string
	}{
		{"configmap-corrupt", "configmap-corrupt.yaml"},
		{"pvc-fill", "pvc-fill.yaml"},
		{"etcd-latency", "etcd-latency.yaml"},
	}

	for _, tc := range clusterFixtures {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			defer cancel()

			fixturePath := filepath.Join(fixtureDir(), tc.fixture)
			obj, err := applyFixture(ctx, c, fixturePath)
			if err != nil {
				t.Fatalf("apply fixture %s: %v", tc.fixture, err)
			}

			name := obj.GetName()
			ns := obj.GetNamespace()
			defer func() {
				if cleanErr := cleanupExperiment(context.Background(), c, ns, name); cleanErr != nil {
					t.Logf("cleanup %s/%s: %v", ns, name, cleanErr)
				}
			}()

			phase, err := waitForPhase(ctx, c, ns, name, []string{"Running", "Completed", "Failed"}, defaultTimeout)
			if err != nil {
				t.Fatalf("wait for phase: %v", err)
			}

			t.Logf("experiment %s reached phase %s", name, phase)
		})
	}
}

func TestTimeChaos(t *testing.T) {
	c, err := newE2EClient()
	if err != nil {
		t.Fatalf("create e2e client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	fixturePath := filepath.Join(fixtureDir(), "time-skew.yaml")
	obj, err := applyFixture(ctx, c, fixturePath)
	if err != nil {
		t.Fatalf("apply fixture time-skew.yaml: %v", err)
	}

	name := obj.GetName()
	ns := obj.GetNamespace()
	defer func() {
		if cleanErr := cleanupExperiment(context.Background(), c, ns, name); cleanErr != nil {
			t.Logf("cleanup %s/%s: %v", ns, name, cleanErr)
		}
	}()

	phase, err := waitForPhase(ctx, c, ns, name, []string{"Running", "Completed", "Failed"}, defaultTimeout)
	if err != nil {
		t.Fatalf("wait for phase: %v", err)
	}

	t.Logf("experiment %s reached phase %s", name, phase)
}

func TestConcurrentExperiments(t *testing.T) {
	c, err := newE2EClient()
	if err != nil {
		t.Fatalf("create e2e client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	fixturePath := filepath.Join(rootFixtureDir(), "concurrent_experiments.yaml")
	objs, err := applyMultiFixture(ctx, c, fixturePath)
	if err != nil {
		t.Fatalf("apply concurrent fixtures: %v", err)
	}

	defer func() {
		if cleanErr := cleanupByLabel(context.Background(), c, "default", "test=concurrent"); cleanErr != nil {
			t.Logf("cleanup concurrent experiments: %v", cleanErr)
		}
	}()

	for _, obj := range objs {
		name := obj.GetName()
		ns := obj.GetNamespace()
		phase, err := waitForPhase(ctx, c, ns, name, []string{"Running", "Completed", "Failed"}, defaultTimeout)
		if err != nil {
			t.Errorf("concurrent experiment %s: %v", name, err)
			continue
		}
		t.Logf("concurrent experiment %s reached phase %s", name, phase)
	}
}

func TestCRDValidation(t *testing.T) {
	c, err := newE2EClient()
	if err != nil {
		t.Fatalf("create e2e client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fixturePath := filepath.Join(rootFixtureDir(), "invalid_experiment.yaml")
	_, err = applyFixture(ctx, c, fixturePath)
	if err == nil {
		defer func() {
			_ = cleanupExperiment(context.Background(), c, "default", "invalid-experiment")
		}()
		t.Fatal("expected CRD validation to reject invalid experiment, but it was accepted")
	}

	if !k8serrors.IsInvalid(err) && !k8serrors.IsBadRequest(err) {
		t.Logf("experiment rejected with error (expected): %v", err)
	}
}

func TestHelmLifecycle(t *testing.T) {
	c, err := newE2EClient()
	if err != nil {
		t.Fatalf("create e2e client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err = c.clientset.AppsV1().Deployments("chaosplane-system").Get(ctx, "chaosplane-operator", metav1.GetOptions{})
	if err != nil {
		t.Skipf("chaosplane operator deployment not found, skipping helm lifecycle test: %v", err)
	}

	deploy, err := c.clientset.AppsV1().Deployments("chaosplane-system").Get(ctx, "chaosplane-operator", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get operator deployment: %v", err)
	}

	if deploy.Status.ReadyReplicas < 1 {
		t.Fatalf("operator has %d ready replicas, expected >= 1", deploy.Status.ReadyReplicas)
	}

	t.Logf("operator deployment has %d ready replicas", deploy.Status.ReadyReplicas)
}

func TestDaemonSetRestart(t *testing.T) {
	c, err := newE2EClient()
	if err != nil {
		t.Fatalf("create e2e client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fixturePath := filepath.Join(rootFixtureDir(), "experiment_long_running.yaml")
	obj, err := applyFixture(ctx, c, fixturePath)
	if err != nil {
		t.Fatalf("apply long-running fixture: %v", err)
	}

	name := obj.GetName()
	ns := obj.GetNamespace()
	defer func() {
		if cleanErr := cleanupExperiment(context.Background(), c, ns, name); cleanErr != nil {
			t.Logf("cleanup %s/%s: %v", ns, name, cleanErr)
		}
	}()

	_, err = waitForPhase(ctx, c, ns, name, []string{"Running"}, defaultTimeout)
	if err != nil {
		t.Fatalf("wait for Running: %v", err)
	}

	ds, err := c.clientset.AppsV1().DaemonSets("chaosplane-system").Get(ctx, "chaosplane-daemon", metav1.GetOptions{})
	if err != nil {
		t.Skipf("chaosplane daemon daemonset not found, skipping restart test: %v", err)
	}

	ds.Spec.Template.ObjectMeta.Annotations = map[string]string{
		"kubectl.kubernetes.io/restartedAt": time.Now().Format(time.RFC3339),
	}
	_, err = c.clientset.AppsV1().DaemonSets("chaosplane-system").Update(ctx, ds, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("restart daemonset: %v", err)
	}

	t.Log("daemonset restarted, waiting for experiment to reach terminal phase")

	phase, err := waitForPhase(ctx, c, ns, name, []string{"Completed", "Failed", "Aborted"}, 2*time.Minute)
	if err != nil {
		t.Fatalf("wait for terminal phase after restart: %v", err)
	}

	t.Logf("experiment reached phase %s after daemon restart", phase)
}
