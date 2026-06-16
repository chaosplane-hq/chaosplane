package controller_test

import (
	"context"
	"fmt"
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

// envtestSetupHint guides the developer to provision control-plane binaries.
// The suite aborts rather than skips on envtest failure so a broken local
// setup cannot mask controller regressions as a green run.
const envtestSetupHint = `envtest failed to start. The control-plane binaries (kube-apiserver, etcd)
are missing or unusable.

Run the controller suite via the Makefile, which provisions them automatically:

    make test

or set KUBEBUILDER_ASSETS manually:

    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.20
    export KUBEBUILDER_ASSETS="$(setup-envtest use 1.32.0 -p path)"
    go test ./internal/controller/...

underlying error: %v`

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
		fmt.Fprintf(os.Stderr, envtestSetupHint+"\n", err)
		os.Exit(1)
	}

	if err := v1alpha1.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "add v1alpha1 to scheme: %v\n", err)
		os.Exit(1)
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create k8s client: %v\n", err)
		os.Exit(1)
	}

	_ = ctrl.SetupSignalHandler()

	code := m.Run()

	_ = testEnv.Stop()

	os.Exit(code)
}
