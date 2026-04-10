package controller_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	testCtx   context.Context
	cancel    context.CancelFunc
	scheme    = runtime.NewScheme()
)

func TestMain(m *testing.M) {
	testCtx, cancel = context.WithCancel(context.Background())
	defer cancel()

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		cfg = nil
		os.Exit(m.Run())
	}

	_ = v1alpha1.AddToScheme(scheme)

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		cfg = nil
		os.Exit(m.Run())
	}

	_ = ctrl.SetupSignalHandler()

	code := m.Run()

	_ = testEnv.Stop()

	os.Exit(code)
}
