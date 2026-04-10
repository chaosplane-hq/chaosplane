package pod

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

var _ executor.Executor = (*HTTPDelayExecutor)(nil)

type HTTPDelayExecutor struct {
	Logger        *slog.Logger
	Client        client.Client
	DaemonFactory DaemonClientFactory

	mu           sync.Mutex
	executionIDs map[string][]nodeExecution
}

func NewHTTPDelayExecutor(logger *slog.Logger, c client.Client, df DaemonClientFactory) *HTTPDelayExecutor {
	return &HTTPDelayExecutor{
		Logger:        logger,
		Client:        c,
		DaemonFactory: df,
		executionIDs:  make(map[string][]nodeExecution),
	}
}

func (e *HTTPDelayExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	pods, err := ResolveTargetPods(ctx, e.Client, exp.Spec.Target)
	if err != nil {
		return fmt.Errorf("pod-http-delay: resolve targets: %w", err)
	}

	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("pod-http-delay: %w", err)
	}

	var port int32
	if v, ok := params["port"]; ok {
		p, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return fmt.Errorf("pod-http-delay: invalid port %q: %w", v, err)
		}
		port = int32(p)
	}

	expKey := string(exp.UID)
	for _, p := range pods {
		endpoint := ResolveDaemonEndpoint(p.Spec.NodeName)
		dc, err := e.DaemonFactory(endpoint)
		if err != nil {
			return fmt.Errorf("pod-http-delay: daemon client for node %s: %w", p.Spec.NodeName, err)
		}

		e.Logger.InfoContext(ctx, "applying http delay", "pod", p.Name, "node", p.Spec.NodeName)
		resp, err := dc.ExecHTTPChaos(ctx, &daemonv1.HTTPChaosRequest{
			ExperimentId: expKey,
			Action:       "delay",
			Port:         port,
			Parameters: map[string]string{
				"delay":        params["delay"],
				"podName":      p.Name,
				"podNamespace": p.Namespace,
			},
		})
		if err != nil {
			return fmt.Errorf("pod-http-delay: exec on pod %s/%s: %w", p.Namespace, p.Name, err)
		}
		if !resp.Success {
			return fmt.Errorf("pod-http-delay: daemon failure for %s/%s: %s", p.Namespace, p.Name, resp.Message)
		}

		e.mu.Lock()
		e.executionIDs[expKey] = append(e.executionIDs[expKey], nodeExecution{nodeName: p.Spec.NodeName, executionID: resp.ExecutionId})
		e.mu.Unlock()
	}
	return nil
}

func (e *HTTPDelayExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	expKey := string(exp.UID)
	e.mu.Lock()
	execs := e.executionIDs[expKey]
	delete(e.executionIDs, expKey)
	e.mu.Unlock()

	for _, ne := range execs {
		endpoint := ResolveDaemonEndpoint(ne.nodeName)
		dc, err := e.DaemonFactory(endpoint)
		if err != nil {
			return fmt.Errorf("pod-http-delay: rollback daemon client for node %s: %w", ne.nodeName, err)
		}
		if _, err := dc.CancelChaos(ctx, &daemonv1.CancelRequest{ExecutionId: ne.executionID}); err != nil {
			return fmt.Errorf("pod-http-delay: cancel execution %s: %w", ne.executionID, err)
		}
	}
	return nil
}

func (e *HTTPDelayExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if exp.Spec.Target.Namespace == "" {
		return fmt.Errorf("pod-http-delay: target namespace is required")
	}
	if exp.Spec.Target.LabelSelector == nil && len(exp.Spec.Target.Names) == 0 {
		return fmt.Errorf("pod-http-delay: target must specify names or labelSelector")
	}
	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("pod-http-delay: %w", err)
	}
	if params["port"] == "" {
		return fmt.Errorf("pod-http-delay: port parameter is required")
	}
	if params["delay"] == "" {
		return fmt.Errorf("pod-http-delay: delay parameter is required")
	}
	return nil
}
