package cli

import (
	"fmt"
	"os"
	"path/filepath"

	chaosv1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var scheme = runtime.NewScheme()

func init() {
	_ = chaosv1alpha1.AddToScheme(scheme)
}

// ResolveKubeconfig returns the kubeconfig path from flag, env, or default.
func ResolveKubeconfig(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("KUBECONFIG"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}

// NewK8sClient creates a controller-runtime client from a kubeconfig path.
func NewK8sClient(kubeconfig string) (client.Client, error) {
	path := ResolveKubeconfig(kubeconfig)
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}
	return c, nil
}

// NewDynamicClient creates a dynamic client from a kubeconfig path.
func NewDynamicClient(kubeconfig string) (dynamic.Interface, error) {
	path := ResolveKubeconfig(kubeconfig)
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}
	return dc, nil
}
