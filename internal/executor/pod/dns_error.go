package pod

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

var _ executor.Executor = (*DNSErrorExecutor)(nil)

type DNSErrorExecutor struct {
	Logger        *slog.Logger
	Client        client.Client
	DaemonFactory DaemonClientFactory

	mu           sync.Mutex
	executionIDs map[string][]nodeExecution
}

func NewDNSErrorExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *DNSErrorExecutor {
	return &DNSErrorExecutor{
		Logger:        logger,
		Client:        c,
		DaemonFactory: df,
		executionIDs:  make(map[string][]nodeExecution),
	}
}

func (e *DNSErrorExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	pods, err := ResolveTargetPods(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("pod-dns-error: resolve targets: %w", err)
	}

	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("pod-dns-error: %w", err)
	}

	expKey := string(exp.UID)
	for _, p := range pods {
		endpoint := ResolveDaemonEndpoint(p.Spec.NodeName)
		dc, err := e.DaemonFactory(endpoint)
		if err != nil {
			return fmt.Errorf("pod-dns-error: daemon client for node %s: %w", p.Spec.NodeName, err)
		}

		e.Logger.InfoContext(ctx, "applying dns error", "pod", p.Name, "node", p.Spec.NodeName)
		resp, err := dc.ExecDNSChaos(ctx, &daemonv1.DNSChaosRequest{
			ExperimentId: expKey,
			Action:       "error",
			Parameters: map[string]string{
				"domains":      params["domains"],
				"errorType":    params["errorType"],
				"podName":      p.Name,
				"podNamespace": p.Namespace,
			},
		})
		if err != nil {
			return fmt.Errorf("pod-dns-error: exec on pod %s/%s: %w", p.Namespace, p.Name, err)
		}
		if !resp.Success {
			return fmt.Errorf("pod-dns-error: daemon failure for %s/%s: %s", p.Namespace, p.Name, resp.Message)
		}

		e.mu.Lock()
		e.executionIDs[expKey] = append(e.executionIDs[expKey], nodeExecution{nodeName: p.Spec.NodeName, executionID: resp.ExecutionId})
		e.mu.Unlock()
	}
	return nil
}

func (e *DNSErrorExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	expKey := string(exp.UID)
	e.mu.Lock()
	execs := e.executionIDs[expKey]
	delete(e.executionIDs, expKey)
	e.mu.Unlock()

	for _, ne := range execs {
		endpoint := ResolveDaemonEndpoint(ne.nodeName)
		dc, err := e.DaemonFactory(endpoint)
		if err != nil {
			return fmt.Errorf("pod-dns-error: rollback daemon client for node %s: %w", ne.nodeName, err)
		}
		if _, err := dc.CancelChaos(ctx, &daemonv1.CancelRequest{ExecutionId: ne.executionID}); err != nil {
			return fmt.Errorf("pod-dns-error: cancel execution %s: %w", ne.executionID, err)
		}
	}
	return nil
}

func (e *DNSErrorExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if exp.Spec.Target.Namespace == "" {
		return fmt.Errorf("pod-dns-error: target namespace is required")
	}
	if exp.Spec.Target.LabelSelector == nil && len(exp.Spec.Target.Names) == 0 {
		return fmt.Errorf("pod-dns-error: target must specify names or labelSelector")
	}
	return nil
}
