package main

import (
	"log/slog"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/controller"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
	"github.com/chaosplane-hq/chaosplane/internal/webhook"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: ":8081",
	})
	if err != nil {
		logger.Error("unable to create manager", "error", err)
		os.Exit(1)
	}

	registry := executor.NewRegistry()

	k8sClient := mgr.GetClient()
	restConfig := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		logger.Error("unable to create kubernetes clientset", "error", err)
		os.Exit(1)
	}
	daemonFactory := pod.DefaultDaemonClientFactory

	registry.MustRegister("pod-kill", pod.NewKillExecutor(logger, k8sClient, clientset))
	registry.MustRegister("container-kill", pod.NewContainerKillExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-cpu-stress", pod.NewCPUStressExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-memory-stress", pod.NewMemoryStressExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-io-stress", pod.NewIOStressExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-dns-error", pod.NewDNSErrorExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-http-abort", pod.NewHTTPAbortExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-http-delay", pod.NewHTTPDelayExecutor(logger, k8sClient, daemonFactory))

	reconciler := &controller.ExperimentReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("chaosplane-controller"),
		Registry: registry,
		Logger:   logger,
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		logger.Error("unable to setup controller", "error", err)
		os.Exit(1)
	}

	webhookServer := mgr.GetWebhookServer()
	webhookServer.Register("/validate-chaosexperiment", webhook.NewBlastRadiusWebhook())

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logger.Error("unable to set up health check", "error", err)
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logger.Error("unable to set up ready check", "error", err)
		os.Exit(1)
	}

	logger.Info("starting operator")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error("operator exited with error", "error", err)
		os.Exit(1)
	}
}
