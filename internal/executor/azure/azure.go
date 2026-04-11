package azure

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

func validateAzureParams(exp *v1alpha1.ChaosExperiment, actionType string, requiredParams ...string) (map[string]string, error) {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", actionType, err)
	}
	if params["subscriptionId"] == "" {
		return nil, fmt.Errorf("%s: subscriptionId parameter is required", actionType)
	}
	if params["resourceGroup"] == "" {
		return nil, fmt.Errorf("%s: resourceGroup parameter is required", actionType)
	}
	for _, p := range requiredParams {
		if params[p] == "" {
			return nil, fmt.Errorf("%s: %s parameter is required", actionType, p)
		}
	}
	return params, nil
}

var _ executor.Executor = (*VMStopExecutor)(nil)

type VMStopExecutor struct {
	Logger *slog.Logger
	state  map[string][]string
}

func NewVMStopExecutor(logger *slog.Logger) *VMStopExecutor {
	return &VMStopExecutor{Logger: logger, state: make(map[string][]string)}
}

func (e *VMStopExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAzureParams(exp, "azure-vm-stop", "vmName")
	if err != nil {
		return err
	}
	e.state[string(exp.UID)] = []string{params["vmName"]}
	e.Logger.Info("azure-vm-stop: deallocating VM", "subscription", params["subscriptionId"], "rg", params["resourceGroup"], "vm", params["vmName"])
	return nil
}

func (e *VMStopExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	vms := e.state[string(exp.UID)]
	delete(e.state, string(exp.UID))
	e.Logger.Info("azure-vm-stop: starting VMs back", "vms", vms)
	return nil
}

func (e *VMStopExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAzureParams(exp, "azure-vm-stop", "vmName")
	return err
}

var _ executor.Executor = (*AKSNodePoolScaleExecutor)(nil)

type AKSNodePoolScaleExecutor struct {
	Logger *slog.Logger
}

func NewAKSNodePoolScaleExecutor(logger *slog.Logger) *AKSNodePoolScaleExecutor {
	return &AKSNodePoolScaleExecutor{Logger: logger}
}

func (e *AKSNodePoolScaleExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAzureParams(exp, "azure-aks-scale", "clusterName", "nodePool", "targetCount")
	if err != nil {
		return err
	}
	e.Logger.Info("azure-aks-scale: scaling node pool", "cluster", params["clusterName"], "pool", params["nodePool"], "target", params["targetCount"])
	return nil
}

func (e *AKSNodePoolScaleExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *AKSNodePoolScaleExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAzureParams(exp, "azure-aks-scale", "clusterName", "nodePool", "targetCount")
	return err
}

var _ executor.Executor = (*CosmosDBFailoverExecutor)(nil)

type CosmosDBFailoverExecutor struct {
	Logger *slog.Logger
}

func NewCosmosDBFailoverExecutor(logger *slog.Logger) *CosmosDBFailoverExecutor {
	return &CosmosDBFailoverExecutor{Logger: logger}
}

func (e *CosmosDBFailoverExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAzureParams(exp, "azure-cosmosdb-failover", "accountName", "targetRegion")
	if err != nil {
		return err
	}
	e.Logger.Info("azure-cosmosdb-failover: triggering failover", "account", params["accountName"], "targetRegion", params["targetRegion"])
	return nil
}

func (e *CosmosDBFailoverExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *CosmosDBFailoverExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAzureParams(exp, "azure-cosmosdb-failover", "accountName", "targetRegion")
	return err
}

var _ = strings.TrimSpace
