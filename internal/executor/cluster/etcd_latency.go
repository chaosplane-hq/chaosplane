package cluster

import (
	"context"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
	"github.com/chaosplane-hq/chaosplane/internal/executor/network"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

var _ executor.Executor = (*EtcdLatencyExecutor)(nil)

// DaemonClientFactory aliases the shared daemon factory so the operator wires
// EtcdLatency the same way as the network/pod executors.
type DaemonClientFactory = pod.DaemonClientFactory

const (
	defaultEtcdNamespace = "kube-system"
	defaultEtcdSelector  = "component=etcd"
)

// EtcdLatencyExecutor injects network latency on the etcd pods' traffic,
// degrading apiserver-to-etcd round trips. It reuses the network-delay daemon
// path (tc netem on the pod's host-side veth) against the etcd pods.
//
// Feasibility constraint (honest): this only works where etcd runs as a Pod the
// operator can see and whose traffic flows through a per-pod veth, i.e.
// self-hosted clusters (kubeadm, kind) running etcd as a static pod on a CNI
// network. It does NOT work on managed control planes (EKS/GKE/AKS) where etcd
// is fully hidden, and it has no effect if etcd runs with hostNetwork (no
// dedicated veth for the daemon to attach netem to). Validate cannot detect the
// managed/hostNetwork case ahead of time; in those environments target
// resolution or the daemon attach will fail loudly rather than fake success.
type EtcdLatencyExecutor struct {
	Logger *slog.Logger
	Client client.Client
	delay  *network.DelayExecutor
}

func NewEtcdLatencyExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *EtcdLatencyExecutor {
	return &EtcdLatencyExecutor{
		Logger: logger,
		Client: c,
		delay:  network.NewDelayExecutor(logger, c, df),
	}
}

// etcdExperiment returns a copy of exp whose target points at the etcd pods, so
// the underlying network-delay executor faults etcd specifically. The UID is
// preserved so the delay executor's rollback bookkeeping keys match.
func etcdExperiment(exp *v1alpha1.ChaosExperiment) (*v1alpha1.ChaosExperiment, error) {
	ns := exp.Spec.Target.Namespace
	if ns == "" {
		ns = defaultEtcdNamespace
	}

	clone := exp.DeepCopy()
	clone.Spec.Target = v1alpha1.TargetSpec{
		Kind:      "Pod",
		Namespace: ns,
	}
	if len(exp.Spec.Target.Names) > 0 {
		clone.Spec.Target.Names = exp.Spec.Target.Names
	} else if exp.Spec.Target.LabelSelector != nil {
		clone.Spec.Target.LabelSelector = exp.Spec.Target.LabelSelector
	} else {
		sel, err := metav1.ParseToLabelSelector(defaultEtcdSelector)
		if err != nil {
			return nil, fmt.Errorf("etcd-latency: parse default selector: %w", err)
		}
		clone.Spec.Target.LabelSelector = sel
	}

	if clone.Spec.Action.Parameters.Raw == nil {
		return nil, fmt.Errorf("etcd-latency: latency parameter is required")
	}
	return clone, nil
}

func (e *EtcdLatencyExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	target, err := etcdExperiment(exp)
	if err != nil {
		return err
	}
	if err := e.delay.Execute(ctx, target); err != nil {
		return fmt.Errorf("etcd-latency: %w", err)
	}
	e.Logger.InfoContext(ctx, "etcd-latency: applied network delay to etcd pods", "namespace", target.Spec.Target.Namespace)
	return nil
}

func (e *EtcdLatencyExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	target, err := etcdExperiment(exp)
	if err != nil {
		return err
	}
	if err := e.delay.Rollback(ctx, target); err != nil {
		return fmt.Errorf("etcd-latency: %w", err)
	}
	return nil
}

func (e *EtcdLatencyExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("etcd-latency: %w", err)
	}
	if params["latency"] == "" {
		return fmt.Errorf("etcd-latency: latency parameter is required")
	}
	return nil
}
