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
	awsexec "github.com/chaosplane-hq/chaosplane/internal/executor/aws"
	azureexec "github.com/chaosplane-hq/chaosplane/internal/executor/azure"
	gcpexec "github.com/chaosplane-hq/chaosplane/internal/executor/gcp"
	"github.com/chaosplane-hq/chaosplane/internal/executor/network"
	"github.com/chaosplane-hq/chaosplane/internal/executor/node"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
	"github.com/chaosplane-hq/chaosplane/internal/executor/stress"
	vmexec "github.com/chaosplane-hq/chaosplane/internal/executor/vm"
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
	nodeDaemonFactory := node.DaemonClientFactory(node.DefaultDaemonClientFactory)
	stressDaemonFactory := stress.DaemonClientFactory(stress.DefaultDaemonClientFactory)

	registry.MustRegister("pod-kill", pod.NewKillExecutor(logger, k8sClient, clientset))
	registry.MustRegister("container-kill", pod.NewContainerKillExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-cpu-stress", pod.NewCPUStressExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-memory-stress", pod.NewMemoryStressExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-io-stress", pod.NewIOStressExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-dns-error", pod.NewDNSErrorExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-http-abort", pod.NewHTTPAbortExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("pod-http-delay", pod.NewHTTPDelayExecutor(logger, k8sClient, daemonFactory))

	registry.MustRegister("network-delay", network.NewDelayExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("network-loss", network.NewLossExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("network-corrupt", network.NewCorruptExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("network-duplicate", network.NewDuplicateExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("network-partition", network.NewPartitionExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("network-bandwidth", network.NewBandwidthExecutor(logger, k8sClient, daemonFactory))

	registry.MustRegister("node-drain", node.NewDrainExecutor(logger, k8sClient, clientset))
	registry.MustRegister("node-taint", node.NewTaintExecutor(logger, k8sClient))
	registry.MustRegister("node-restart", node.NewRestartExecutor(logger, k8sClient, nodeDaemonFactory))
	registry.MustRegister("node-cpu-stress", node.NewCPUStressExecutor(logger, k8sClient, nodeDaemonFactory))

	registry.MustRegister("stress-cpu", stress.NewCPUExecutor(logger, k8sClient, stressDaemonFactory))
	registry.MustRegister("stress-memory", stress.NewMemoryExecutor(logger, k8sClient, stressDaemonFactory))

	registry.MustRegister("ebpf-network-delay", network.NewEBPFDelayExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("ebpf-network-loss", network.NewEBPFLossExecutor(logger, k8sClient, daemonFactory))
	registry.MustRegister("ebpf-dns-chaos", network.NewEBPFDNSExecutor(logger, k8sClient, daemonFactory))

	registry.MustRegister("aws-ec2-stop", awsexec.NewEC2StopExecutor(logger))
	registry.MustRegister("aws-ec2-terminate", awsexec.NewEC2TerminateExecutor(logger))
	registry.MustRegister("aws-rds-failover", awsexec.NewRDSFailoverExecutor(logger))
	registry.MustRegister("aws-ecs-stop-task", awsexec.NewECSStopTaskExecutor(logger))
	registry.MustRegister("aws-az-failure", awsexec.NewAZFailureExecutor(logger))

	registry.MustRegister("vm-cpu-stress", vmexec.NewCPUStressExecutor(logger))
	registry.MustRegister("vm-memory-stress", vmexec.NewMemoryStressExecutor(logger))
	registry.MustRegister("vm-disk-stress", vmexec.NewDiskStressExecutor(logger))
	registry.MustRegister("vm-network-delay", vmexec.NewNetworkDelayExecutor(logger))
	registry.MustRegister("vm-process-kill", vmexec.NewProcessKillExecutor(logger))
	registry.MustRegister("vm-process-suspend", vmexec.NewProcessSuspendExecutor(logger))

	registry.MustRegister("azure-vm-stop", azureexec.NewVMStopExecutor(logger))
	registry.MustRegister("azure-aks-scale", azureexec.NewAKSNodePoolScaleExecutor(logger))
	registry.MustRegister("azure-cosmosdb-failover", azureexec.NewCosmosDBFailoverExecutor(logger))

	registry.MustRegister("gcp-gke-scale", gcpexec.NewGKENodePoolScaleExecutor(logger))
	registry.MustRegister("gcp-cloudsql-failover", gcpexec.NewCloudSQLFailoverExecutor(logger))
	registry.MustRegister("gcp-cloudrun-stop", gcpexec.NewCloudRunStopExecutor(logger))

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

	workflowReconciler := &controller.WorkflowReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("workflow-controller"),
		Logger:   slog.Default().With("controller", "workflow"),
	}
	if err := workflowReconciler.SetupWithManager(mgr); err != nil {
		logger.Error("unable to setup workflow controller", "error", err)
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
