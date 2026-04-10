package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

type AWSConfig struct {
	Region    string
	RoleARN   string
	AccountID string
}

func configFromParams(params map[string]string) AWSConfig {
	return AWSConfig{
		Region:    params["awsRegion"],
		RoleARN:   params["awsRoleArn"],
		AccountID: params["awsAccountId"],
	}
}

func validateAWSParams(exp *v1alpha1.ChaosExperiment, actionType string, requiredParams ...string) (map[string]string, error) {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", actionType, err)
	}
	if params["awsRegion"] == "" {
		return nil, fmt.Errorf("%s: awsRegion parameter is required", actionType)
	}
	for _, p := range requiredParams {
		if params[p] == "" {
			return nil, fmt.Errorf("%s: %s parameter is required", actionType, p)
		}
	}
	return params, nil
}

var _ executor.Executor = (*EC2StopExecutor)(nil)

type EC2StopExecutor struct {
	Logger *slog.Logger
	state  map[string][]string
}

func NewEC2StopExecutor(logger *slog.Logger) *EC2StopExecutor {
	return &EC2StopExecutor{Logger: logger, state: make(map[string][]string)}
}

func (e *EC2StopExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAWSParams(exp, "aws-ec2-stop", "instanceIds")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	instanceIDs := strings.Split(params["instanceIds"], ",")

	e.Logger.Info("aws-ec2-stop: stopping instances", "region", cfg.Region, "instances", instanceIDs, "roleArn", cfg.RoleARN)
	e.state[string(exp.UID)] = instanceIDs
	return nil
}

func (e *EC2StopExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	instanceIDs := e.state[string(exp.UID)]
	delete(e.state, string(exp.UID))
	e.Logger.Info("aws-ec2-stop: starting instances back", "instances", instanceIDs)
	return nil
}

func (e *EC2StopExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAWSParams(exp, "aws-ec2-stop", "instanceIds")
	return err
}

var _ executor.Executor = (*EC2TerminateExecutor)(nil)

type EC2TerminateExecutor struct {
	Logger *slog.Logger
}

func NewEC2TerminateExecutor(logger *slog.Logger) *EC2TerminateExecutor {
	return &EC2TerminateExecutor{Logger: logger}
}

func (e *EC2TerminateExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAWSParams(exp, "aws-ec2-terminate", "instanceIds")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	instanceIDs := strings.Split(params["instanceIds"], ",")
	e.Logger.Info("aws-ec2-terminate: terminating instances", "region", cfg.Region, "instances", instanceIDs)
	return nil
}

func (e *EC2TerminateExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *EC2TerminateExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAWSParams(exp, "aws-ec2-terminate", "instanceIds")
	return err
}

var _ executor.Executor = (*RDSFailoverExecutor)(nil)

type RDSFailoverExecutor struct {
	Logger *slog.Logger
}

func NewRDSFailoverExecutor(logger *slog.Logger) *RDSFailoverExecutor {
	return &RDSFailoverExecutor{Logger: logger}
}

func (e *RDSFailoverExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAWSParams(exp, "aws-rds-failover", "dbClusterIdentifier")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	e.Logger.Info("aws-rds-failover: triggering failover", "region", cfg.Region, "cluster", params["dbClusterIdentifier"])
	return nil
}

func (e *RDSFailoverExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *RDSFailoverExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAWSParams(exp, "aws-rds-failover", "dbClusterIdentifier")
	return err
}

var _ executor.Executor = (*ECSStopTaskExecutor)(nil)

type ECSStopTaskExecutor struct {
	Logger *slog.Logger
}

func NewECSStopTaskExecutor(logger *slog.Logger) *ECSStopTaskExecutor {
	return &ECSStopTaskExecutor{Logger: logger}
}

func (e *ECSStopTaskExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAWSParams(exp, "aws-ecs-stop-task", "cluster", "taskArn")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	e.Logger.Info("aws-ecs-stop-task: stopping task", "region", cfg.Region, "cluster", params["cluster"], "task", params["taskArn"])
	return nil
}

func (e *ECSStopTaskExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *ECSStopTaskExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAWSParams(exp, "aws-ecs-stop-task", "cluster", "taskArn")
	return err
}

var _ executor.Executor = (*AZFailureExecutor)(nil)

type AZFailureExecutor struct {
	Logger *slog.Logger
}

func NewAZFailureExecutor(logger *slog.Logger) *AZFailureExecutor {
	return &AZFailureExecutor{Logger: logger}
}

func (e *AZFailureExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAWSParams(exp, "aws-az-failure", "availabilityZone")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	e.Logger.Info("aws-az-failure: simulating AZ failure", "region", cfg.Region, "az", params["availabilityZone"])
	return nil
}

func (e *AZFailureExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *AZFailureExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAWSParams(exp, "aws-az-failure", "availabilityZone")
	return err
}
