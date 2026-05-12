//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var chaosExperimentGVR = schema.GroupVersionResource{
	Group:    "chaos.chaosplane.dev",
	Version:  "v1alpha1",
	Resource: "chaosexperiments",
}

type e2eClient struct {
	dynamic   dynamic.Interface
	clientset kubernetes.Interface
}

func newE2EClient() (*e2eClient, error) {
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

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	return &e2eClient{dynamic: dynClient, clientset: clientset}, nil
}

func applyFixture(ctx context.Context, c *e2eClient, fixturePath string) (*unstructured.Unstructured, error) {
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		return nil, fmt.Errorf("read fixture %s: %w", fixturePath, err)
	}

	obj := &unstructured.Unstructured{}
	if err := yaml.NewYAMLOrJSONDecoder(
		bytes.NewReader(data), len(data),
	).Decode(obj); err != nil {
		return nil, fmt.Errorf("decode fixture %s: %w", fixturePath, err)
	}

	ns := obj.GetNamespace()
	if ns == "" {
		ns = "default"
	}

	created, err := c.dynamic.Resource(chaosExperimentGVR).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create experiment from %s: %w", fixturePath, err)
	}
	return created, nil
}

func applyMultiFixture(ctx context.Context, c *e2eClient, fixturePath string) ([]*unstructured.Unstructured, error) {
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		return nil, fmt.Errorf("read fixture %s: %w", fixturePath, err)
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var list unstructured.Unstructured
	if err := decoder.Decode(&list); err != nil {
		return nil, fmt.Errorf("decode list from %s: %w", fixturePath, err)
	}

	items, found, err := unstructured.NestedSlice(list.Object, "items")
	if err != nil || !found {
		return nil, fmt.Errorf("no items in list from %s", fixturePath)
	}

	var results []*unstructured.Unstructured
	for _, item := range items {
		raw, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		obj := &unstructured.Unstructured{Object: raw}
		ns := obj.GetNamespace()
		if ns == "" {
			ns = "default"
		}
		created, err := c.dynamic.Resource(chaosExperimentGVR).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return results, fmt.Errorf("create experiment %s: %w", obj.GetName(), err)
		}
		results = append(results, created)
	}
	return results, nil
}

func waitForPhase(ctx context.Context, c *e2eClient, namespace, name string, phases []string, timeout time.Duration) (string, error) {
	var lastPhase string
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		obj, err := c.dynamic.Resource(chaosExperimentGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		lastPhase = phase
		for _, p := range phases {
			if phase == p {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return lastPhase, fmt.Errorf("waiting for %s/%s to reach %v: last phase=%q: %w", namespace, name, phases, lastPhase, err)
	}
	return lastPhase, nil
}

func cleanupExperiment(ctx context.Context, c *e2eClient, namespace, name string) error {
	err := c.dynamic.Resource(chaosExperimentGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete experiment %s/%s: %w", namespace, name, err)
	}
	return nil
}

func cleanupByLabel(ctx context.Context, c *e2eClient, namespace, labelSelector string) error {
	return c.dynamic.Resource(chaosExperimentGVR).Namespace(namespace).DeleteCollection(
		ctx,
		metav1.DeleteOptions{},
		metav1.ListOptions{LabelSelector: labelSelector},
	)
}

func fixtureDir() string {
	return filepath.Join("..", "fixtures", "e2e")
}

func rootFixtureDir() string {
	return filepath.Join("..", "fixtures")
}
