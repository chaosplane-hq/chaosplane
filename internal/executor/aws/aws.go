package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
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
	client, err := clientFactory(cfg)
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
	client, err := clientFactory(cfg)
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
	client, err := clientFactory(cfg)
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
	client, err := clientFactory(cfg)
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
	client, err := clientFactory(cfg)
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

// azFailureState records what must be undone to restore an isolated AZ: the
// deny-all NACL we created, and the original NACL association each subnet had
// before we replaced it.
type azFailureState struct {
	denyACLID    string
	associations []azReplacedAssoc
}

type azReplacedAssoc struct {
	associationID string
	originalACLID string
}

type AZFailureExecutor struct {
	Logger *slog.Logger

	mu    sync.Mutex
	state map[string]*azFailureState
}

func NewAZFailureExecutor(logger *slog.Logger) *AZFailureExecutor {
	return &AZFailureExecutor{Logger: logger, state: make(map[string]*azFailureState)}
}

// Execute isolates an AZ by creating a deny-all network ACL and pointing every
// subnet in that AZ at it, severing in/out traffic the way a real AZ outage
// would. Each subnet's prior NACL association is recorded so Rollback can
// restore it.
func (e *AZFailureExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAWSParams(exp, "aws-az-failure", "availabilityZone")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	client, err := clientFactory(cfg)
	if err != nil {
		return fmt.Errorf("aws-az-failure: %w", err)
	}
	az := params["availabilityZone"]

	azFilter := "availability-zone"
	subnetsOut, err := client.EC2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2types.Filter{{Name: &azFilter, Values: []string{az}}},
	})
	if err != nil {
		return fmt.Errorf("aws-az-failure: describe subnets: %w", err)
	}
	if len(subnetsOut.Subnets) == 0 {
		return fmt.Errorf("aws-az-failure: no subnets found in AZ %s", az)
	}

	vpcID := aws.ToString(subnetsOut.Subnets[0].VpcId)
	subnetIDs := make([]string, 0, len(subnetsOut.Subnets))
	for _, s := range subnetsOut.Subnets {
		subnetIDs = append(subnetIDs, aws.ToString(s.SubnetId))
	}

	denyACL, err := client.EC2.CreateNetworkAcl(ctx, &ec2.CreateNetworkAclInput{VpcId: &vpcID})
	if err != nil {
		return fmt.Errorf("aws-az-failure: create deny NACL: %w", err)
	}
	denyACLID := aws.ToString(denyACL.NetworkAcl.NetworkAclId)

	st := &azFailureState{denyACLID: denyACLID}

	// A fresh NACL only carries an implicit deny; an explicit deny-all rule 100
	// for all protocols makes the isolation intent unambiguous in both directions.
	cidrAll := "0.0.0.0/0"
	proto := "-1"
	for _, egress := range []bool{false, true} {
		if _, err := client.EC2.CreateNetworkAclEntry(ctx, &ec2.CreateNetworkAclEntryInput{
			NetworkAclId: &denyACLID,
			RuleNumber:   aws.Int32(100),
			Egress:       aws.Bool(egress),
			Protocol:     &proto,
			RuleAction:   ec2types.RuleActionDeny,
			CidrBlock:    &cidrAll,
		}); err != nil {
			_ = e.cleanup(ctx, client, st)
			return fmt.Errorf("aws-az-failure: add deny entry (egress=%v): %w", egress, err)
		}
	}

	assocs, err := e.currentAssociations(ctx, client, subnetIDs)
	if err != nil {
		_ = e.cleanup(ctx, client, st)
		return fmt.Errorf("aws-az-failure: %w", err)
	}

	for _, a := range assocs {
		assocID := a.associationID
		resp, err := client.EC2.ReplaceNetworkAclAssociation(ctx, &ec2.ReplaceNetworkAclAssociationInput{
			AssociationId: &assocID,
			NetworkAclId:  &denyACLID,
		})
		if err != nil {
			_ = e.cleanup(ctx, client, st)
			return fmt.Errorf("aws-az-failure: replace NACL association %s: %w", assocID, err)
		}
		st.associations = append(st.associations, azReplacedAssoc{
			associationID: aws.ToString(resp.NewAssociationId),
			originalACLID: a.originalACLID,
		})
	}

	e.mu.Lock()
	e.state[string(exp.UID)] = st
	e.mu.Unlock()

	e.Logger.Info("aws-az-failure: isolated AZ", "region", cfg.Region, "az", az, "subnets", len(subnetIDs), "denyAcl", denyACLID)
	return nil
}

func (e *AZFailureExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	e.mu.Lock()
	st := e.state[string(exp.UID)]
	delete(e.state, string(exp.UID))
	e.mu.Unlock()
	if st == nil {
		return nil
	}

	params, _ := pod.ParseParameters(exp)
	cfg := configFromParams(params)
	client, err := clientFactory(cfg)
	if err != nil {
		return fmt.Errorf("aws-az-failure rollback: %w", err)
	}
	if err := e.cleanup(ctx, client, st); err != nil {
		return fmt.Errorf("aws-az-failure rollback: %w", err)
	}
	e.Logger.Info("aws-az-failure: restored AZ NACL associations", "denyAcl", st.denyACLID)
	return nil
}

type azCurrentAssoc struct {
	associationID string
	originalACLID string
}

// currentAssociations finds, per subnet, the NACL association ID and the ACL it
// currently points at. ReplaceNetworkAclAssociation operates on the association
// rather than the subnet, so both pieces are needed to swap to deny and back.
func (e *AZFailureExecutor) currentAssociations(ctx context.Context, client *AWSClient, subnetIDs []string) ([]azCurrentAssoc, error) {
	assocFilter := "association.subnet-id"
	out, err := client.EC2.DescribeNetworkAcls(ctx, &ec2.DescribeNetworkAclsInput{
		Filters: []ec2types.Filter{{Name: &assocFilter, Values: subnetIDs}},
	})
	if err != nil {
		return nil, fmt.Errorf("describe network acls: %w", err)
	}

	wanted := make(map[string]bool, len(subnetIDs))
	for _, id := range subnetIDs {
		wanted[id] = true
	}

	var result []azCurrentAssoc
	for _, acl := range out.NetworkAcls {
		for _, assoc := range acl.Associations {
			if wanted[aws.ToString(assoc.SubnetId)] {
				result = append(result, azCurrentAssoc{
					associationID: aws.ToString(assoc.NetworkAclAssociationId),
					originalACLID: aws.ToString(acl.NetworkAclId),
				})
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no NACL associations found for subnets %v", subnetIDs)
	}
	return result, nil
}

// cleanup restores each replaced association to its original ACL, then deletes
// the deny-all NACL. Safe to call partway through Execute on failure.
func (e *AZFailureExecutor) cleanup(ctx context.Context, client *AWSClient, st *azFailureState) error {
	var firstErr error
	for _, a := range st.associations {
		assocID := a.associationID
		aclID := a.originalACLID
		if _, err := client.EC2.ReplaceNetworkAclAssociation(ctx, &ec2.ReplaceNetworkAclAssociationInput{
			AssociationId: &assocID,
			NetworkAclId:  &aclID,
		}); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("restore association %s to acl %s: %w", assocID, aclID, err)
		}
	}
	if st.denyACLID != "" {
		if _, err := client.EC2.DeleteNetworkAcl(ctx, &ec2.DeleteNetworkAclInput{
			NetworkAclId: &st.denyACLID,
		}); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("delete deny NACL %s: %w", st.denyACLID, err)
		}
	}
	return firstErr
}

func (e *AZFailureExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAWSParams(exp, "aws-az-failure", "availabilityZone")
	return err
}
