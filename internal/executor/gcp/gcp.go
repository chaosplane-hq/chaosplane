package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

func validateGCPParams(exp *v1alpha1.ChaosExperiment, actionType string, requiredParams ...string) (map[string]string, error) {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", actionType, err)
	}
	if params["projectId"] == "" {
		return nil, fmt.Errorf("%s: projectId parameter is required", actionType)
	}
	for _, p := range requiredParams {
		if params[p] == "" {
			return nil, fmt.Errorf("%s: %s parameter is required", actionType, p)
		}
	}
	return params, nil
}

var _ executor.Executor = (*GKENodePoolScaleExecutor)(nil)

type GKENodePoolScaleExecutor struct {
	Logger *slog.Logger
}

func NewGKENodePoolScaleExecutor(logger *slog.Logger) *GKENodePoolScaleExecutor {
	return &GKENodePoolScaleExecutor{Logger: logger}
}

func (e *GKENodePoolScaleExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateGCPParams(exp, "gcp-gke-scale", "clusterName", "nodePool", "zone", "targetSize")
	if err != nil {
		return err
	}
	client := NewGCPClient(params)
	size := 0
	fmt.Sscanf(params["targetSize"], "%d", &size)

	if err := client.GKESetNodePoolSize(ctx, params["zone"], params["clusterName"], params["nodePool"], size); err != nil {
		return fmt.Errorf("gcp-gke-scale: %w", err)
	}
	e.Logger.Info("gcp-gke-scale: scaled node pool", "cluster", params["clusterName"], "pool", params["nodePool"], "target", size)
	return nil
}

func (e *GKENodePoolScaleExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *GKENodePoolScaleExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateGCPParams(exp, "gcp-gke-scale", "clusterName", "nodePool", "zone", "targetSize")
	return err
}

var _ executor.Executor = (*CloudSQLFailoverExecutor)(nil)

type CloudSQLFailoverExecutor struct {
	Logger *slog.Logger
}

func NewCloudSQLFailoverExecutor(logger *slog.Logger) *CloudSQLFailoverExecutor {
	return &CloudSQLFailoverExecutor{Logger: logger}
}

func (e *CloudSQLFailoverExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateGCPParams(exp, "gcp-cloudsql-failover", "instanceName")
	if err != nil {
		return err
	}
	client := NewGCPClient(params)

	if err := client.CloudSQLFailover(ctx, params["instanceName"]); err != nil {
		return fmt.Errorf("gcp-cloudsql-failover: %w", err)
	}
	e.Logger.Info("gcp-cloudsql-failover: triggered failover", "project", params["projectId"], "instance", params["instanceName"])
	return nil
}

func (e *CloudSQLFailoverExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *CloudSQLFailoverExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateGCPParams(exp, "gcp-cloudsql-failover", "instanceName")
	return err
}

var _ executor.Executor = (*CloudRunStopExecutor)(nil)

type CloudRunStopExecutor struct {
	Logger *slog.Logger
}

func NewCloudRunStopExecutor(logger *slog.Logger) *CloudRunStopExecutor {
	return &CloudRunStopExecutor{Logger: logger}
}

func (e *CloudRunStopExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateGCPParams(exp, "gcp-cloudrun-stop", "serviceName", "region")
	if err != nil {
		return err
	}
	client := NewGCPClient(params)

	if err := client.CloudRunUpdateTraffic(ctx, params["region"], params["serviceName"], 0); err != nil {
		return fmt.Errorf("gcp-cloudrun-stop: %w", err)
	}
	e.Logger.Info("gcp-cloudrun-stop: scaled to zero", "project", params["projectId"], "service", params["serviceName"], "region", params["region"])
	return nil
}

func (e *CloudRunStopExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *CloudRunStopExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateGCPParams(exp, "gcp-cloudrun-stop", "serviceName", "region")
	return err
}

var _ = strings.TrimSpace
