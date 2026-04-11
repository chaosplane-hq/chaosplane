package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/rds"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

type AWSConfig struct {
	Region       string
	RoleARN      string
	AccountID    string
	AccessKey    string
	SecretKey    string
	SessionToken string
}

func configFromParams(params map[string]string) AWSConfig {
	return AWSConfig{
		Region:       params["awsRegion"],
		RoleARN:      params["awsRoleArn"],
		AccountID:    params["awsAccountId"],
		AccessKey:    params["awsAccessKey"],
		SecretKey:    params["awsSecretKey"],
		SessionToken: params["awsSessionToken"],
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

func trimIDs(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
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
	client, err := NewAWSClient(cfg)
	if err != nil {
		return fmt.Errorf("aws-ec2-stop: %w", err)
	}
	instanceIDs := trimIDs(params["instanceIds"])

	if _, err := client.EC2.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: instanceIDs,
	}); err != nil {
		return fmt.Errorf("aws-ec2-stop: %w", err)
	}

	e.Logger.Info("aws-ec2-stop: stopped instances", "region", cfg.Region, "instances", instanceIDs)
	e.state[string(exp.UID)] = instanceIDs
	return nil
}

func (e *EC2StopExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	instanceIDs := e.state[string(exp.UID)]
	delete(e.state, string(exp.UID))
	if len(instanceIDs) == 0 {
		return nil
	}

	params, _ := pod.ParseParameters(exp)
	cfg := configFromParams(params)
	client, err := NewAWSClient(cfg)
	if err != nil {
		return fmt.Errorf("aws-ec2-stop rollback: %w", err)
	}

	if _, err := client.EC2.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: instanceIDs,
	}); err != nil {
		return fmt.Errorf("aws-ec2-stop rollback: %w", err)
	}
	e.Logger.Info("aws-ec2-stop: started instances back", "instances", instanceIDs)
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
	client, err := NewAWSClient(cfg)
	if err != nil {
		return fmt.Errorf("aws-ec2-terminate: %w", err)
	}
	instanceIDs := trimIDs(params["instanceIds"])

	if _, err := client.EC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: instanceIDs,
	}); err != nil {
		return fmt.Errorf("aws-ec2-terminate: %w", err)
	}
	e.Logger.Info("aws-ec2-terminate: terminated instances", "region", cfg.Region, "instances", instanceIDs)
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
	client, err := NewAWSClient(cfg)
	if err != nil {
		return fmt.Errorf("aws-rds-failover: %w", err)
	}

	clusterID := params["dbClusterIdentifier"]
	input := &rds.FailoverDBClusterInput{
		DBClusterIdentifier: &clusterID,
	}
	if target := params["targetDBInstanceIdentifier"]; target != "" {
		input.TargetDBInstanceIdentifier = &target
	}
	if _, err := client.RDS.FailoverDBCluster(ctx, input); err != nil {
		return fmt.Errorf("aws-rds-failover: %w", err)
	}
	e.Logger.Info("aws-rds-failover: triggered failover", "region", cfg.Region, "cluster", clusterID)
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
	client, err := NewAWSClient(cfg)
	if err != nil {
		return fmt.Errorf("aws-ecs-stop-task: %w", err)
	}

	cluster := params["cluster"]
	task := params["taskArn"]
	reason := "ChaosPlane experiment: " + exp.Name
	if _, err := client.ECS.StopTask(ctx, &ecs.StopTaskInput{
		Cluster: &cluster,
		Task:    &task,
		Reason:  &reason,
	}); err != nil {
		return fmt.Errorf("aws-ecs-stop-task: %w", err)
	}
	e.Logger.Info("aws-ecs-stop-task: stopped task", "region", cfg.Region, "cluster", cluster, "task", task)
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
	client, err := NewAWSClient(cfg)
	if err != nil {
		return fmt.Errorf("aws-az-failure: %w", err)
	}
	az := params["availabilityZone"]

	filterName := "availability-zone"
	subnets, err := client.EC2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{Name: &filterName, Values: []string{az}},
		},
	})
	if err != nil {
		return fmt.Errorf("aws-az-failure: describe subnets: %w", err)
	}

	e.Logger.Info("aws-az-failure: simulating AZ failure", "region", cfg.Region, "az", az, "subnetsFound", len(subnets.Subnets))
	return nil
}

func (e *AZFailureExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *AZFailureExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAWSParams(exp, "aws-az-failure", "availabilityZone")
	return err
}
