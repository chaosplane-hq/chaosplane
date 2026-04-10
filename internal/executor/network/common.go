package network

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

type DaemonClientFactory = pod.DaemonClientFactory

type baseNetworkExecutor struct {
	Logger        *slog.Logger
	Client        client.Client
	DaemonFactory DaemonClientFactory

	mu           sync.Mutex
	executionIDs map[string][]executionRecord
}

type executionRecord struct {
	endpoint    string
	executionID string
}

func (b *baseNetworkExecutor) storeExecution(expUID, endpoint, execID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.executionIDs == nil {
		b.executionIDs = make(map[string][]executionRecord)
	}
	b.executionIDs[expUID] = append(b.executionIDs[expUID], executionRecord{
		endpoint:    endpoint,
		executionID: execID,
	})
}

func (b *baseNetworkExecutor) rollbackExecutions(ctx context.Context, expUID string) error {
	b.mu.Lock()
	records := b.executionIDs[expUID]
	delete(b.executionIDs, expUID)
	b.mu.Unlock()

	var errs []error
	for _, rec := range records {
		dc, err := b.DaemonFactory(rec.endpoint)
		if err != nil {
			errs = append(errs, fmt.Errorf("daemon client for %s: %w", rec.endpoint, err))
			continue
		}
		resp, err := dc.CancelChaos(ctx, &daemonv1.CancelRequest{ExecutionId: rec.executionID})
		if err != nil {
			errs = append(errs, fmt.Errorf("cancel %s: %w", rec.executionID, err))
			continue
		}
		if !resp.Success {
			errs = append(errs, fmt.Errorf("cancel %s: %s", rec.executionID, resp.Message))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("rollback errors: %v", errs)
	}
	return nil
}

func (b *baseNetworkExecutor) execNetworkChaos(ctx context.Context, exp *v1alpha1.ChaosExperiment, action string, params map[string]string, prefix string) error {
	pods, err := pod.ResolveTargetPods(ctx, b.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("%s: resolve targets: %w", prefix, err)
	}

	for _, p := range pods {
		endpoint := pod.ResolveDaemonEndpoint(p.Spec.NodeName)
		dc, err := b.DaemonFactory(endpoint)
		if err != nil {
			return fmt.Errorf("%s: daemon client for node %s: %w", prefix, p.Spec.NodeName, err)
		}

		b.Logger.InfoContext(ctx, "applying network chaos", "action", action, "pod", p.Name, "node", p.Spec.NodeName)
		resp, err := dc.ExecNetworkChaos(ctx, &daemonv1.NetworkChaosRequest{
			ExperimentId: string(exp.UID),
			Action:       action,
			TargetIface:  fmt.Sprintf("veth_%s", p.Name),
			Parameters:   params,
		})
		if err != nil {
			return fmt.Errorf("%s: exec on pod %s/%s: %w", prefix, p.Namespace, p.Name, err)
		}
		if !resp.Success {
			return fmt.Errorf("%s: daemon failure for %s/%s: %s", prefix, p.Namespace, p.Name, resp.Message)
		}
		b.storeExecution(string(exp.UID), endpoint, resp.ExecutionId)
	}
	return nil
}

func validateTarget(exp *v1alpha1.ChaosExperiment, prefix string) error {
	if exp.Spec.Target.Namespace == "" {
		return fmt.Errorf("%s: target namespace is required", prefix)
	}
	if exp.Spec.Target.LabelSelector == nil && len(exp.Spec.Target.Names) == 0 {
		return fmt.Errorf("%s: target must specify names or labelSelector", prefix)
	}
	return nil
}
