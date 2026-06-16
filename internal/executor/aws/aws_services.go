package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

var _ executor.Executor = (*ElastiCacheFailoverExecutor)(nil)

type ElastiCacheFailoverExecutor struct {
	Logger *slog.Logger
}

func NewElastiCacheFailoverExecutor(logger *slog.Logger) *ElastiCacheFailoverExecutor {
	return &ElastiCacheFailoverExecutor{Logger: logger}
}

func (e *ElastiCacheFailoverExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAWSParams(exp, "aws-elasticache-failover", "replicationGroupId", "nodeGroupId")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	client, err := clientFactory(cfg)
	if err != nil {
		return fmt.Errorf("aws-elasticache-failover: %w", err)
	}

	rgID := params["replicationGroupId"]
	ngID := params["nodeGroupId"]
	if _, err := client.ElastiCache.TestFailover(ctx, &elasticache.TestFailoverInput{
		ReplicationGroupId: &rgID,
		NodeGroupId:        &ngID,
	}); err != nil {
		return fmt.Errorf("aws-elasticache-failover: %w", err)
	}
	e.Logger.Info("aws-elasticache-failover: triggered failover", "region", cfg.Region, "replicationGroup", rgID, "nodeGroup", ngID)
	return nil
}

// Rollback is a no-op: TestFailover promotes a replica and ElastiCache manages
// re-replication automatically, so there is no original state to restore.
func (e *ElastiCacheFailoverExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *ElastiCacheFailoverExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAWSParams(exp, "aws-elasticache-failover", "replicationGroupId", "nodeGroupId")
	return err
}

var _ executor.Executor = (*EKSNodegroupScaleExecutor)(nil)

type EKSNodegroupScaleExecutor struct {
	Logger *slog.Logger

	mu    sync.Mutex
	state map[string]*ekstypes.NodegroupScalingConfig
}

func NewEKSNodegroupScaleExecutor(logger *slog.Logger) *EKSNodegroupScaleExecutor {
	return &EKSNodegroupScaleExecutor{Logger: logger, state: make(map[string]*ekstypes.NodegroupScalingConfig)}
}

// Execute snapshots the nodegroup's current scaling config, then scales it down
// to simulate capacity loss. The snapshot lets Rollback restore the exact prior
// min/max/desired sizes.
func (e *EKSNodegroupScaleExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAWSParams(exp, "aws-eks-nodegroup-scale", "clusterName", "nodegroupName", "desiredSize")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	client, err := clientFactory(cfg)
	if err != nil {
		return fmt.Errorf("aws-eks-nodegroup-scale: %w", err)
	}

	cluster := params["clusterName"]
	ng := params["nodegroupName"]
	desired, err := strconv.ParseInt(params["desiredSize"], 10, 32)
	if err != nil {
		return fmt.Errorf("aws-eks-nodegroup-scale: invalid desiredSize %q: %w", params["desiredSize"], err)
	}

	desc, err := client.EKS.DescribeNodegroup(ctx, &eks.DescribeNodegroupInput{
		ClusterName:   &cluster,
		NodegroupName: &ng,
	})
	if err != nil {
		return fmt.Errorf("aws-eks-nodegroup-scale: describe nodegroup: %w", err)
	}
	if desc.Nodegroup == nil || desc.Nodegroup.ScalingConfig == nil {
		return fmt.Errorf("aws-eks-nodegroup-scale: nodegroup %s has no scaling config", ng)
	}
	orig := desc.Nodegroup.ScalingConfig

	target := int32(desired)
	// minSize must not exceed desired, so clamp it down when scaling below the
	// existing floor; otherwise the API rejects the update.
	newMin := aws.ToInt32(orig.MinSize)
	if newMin > target {
		newMin = target
	}
	if _, err := client.EKS.UpdateNodegroupConfig(ctx, &eks.UpdateNodegroupConfigInput{
		ClusterName:   &cluster,
		NodegroupName: &ng,
		ScalingConfig: &ekstypes.NodegroupScalingConfig{
			DesiredSize: &target,
			MinSize:     &newMin,
			MaxSize:     orig.MaxSize,
		},
	}); err != nil {
		return fmt.Errorf("aws-eks-nodegroup-scale: update config: %w", err)
	}

	e.mu.Lock()
	e.state[string(exp.UID)] = orig
	e.mu.Unlock()

	e.Logger.Info("aws-eks-nodegroup-scale: scaled nodegroup", "region", cfg.Region, "cluster", cluster, "nodegroup", ng, "desired", target)
	return nil
}

func (e *EKSNodegroupScaleExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	e.mu.Lock()
	orig := e.state[string(exp.UID)]
	delete(e.state, string(exp.UID))
	e.mu.Unlock()
	if orig == nil {
		return nil
	}

	params, _ := pod.ParseParameters(exp)
	cfg := configFromParams(params)
	client, err := clientFactory(cfg)
	if err != nil {
		return fmt.Errorf("aws-eks-nodegroup-scale rollback: %w", err)
	}
	cluster := params["clusterName"]
	ng := params["nodegroupName"]
	if _, err := client.EKS.UpdateNodegroupConfig(ctx, &eks.UpdateNodegroupConfigInput{
		ClusterName:   &cluster,
		NodegroupName: &ng,
		ScalingConfig: &ekstypes.NodegroupScalingConfig{
			DesiredSize: orig.DesiredSize,
			MinSize:     orig.MinSize,
			MaxSize:     orig.MaxSize,
		},
	}); err != nil {
		return fmt.Errorf("aws-eks-nodegroup-scale rollback: %w", err)
	}
	e.Logger.Info("aws-eks-nodegroup-scale: restored nodegroup scaling", "cluster", cluster, "nodegroup", ng)
	return nil
}

func (e *EKSNodegroupScaleExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAWSParams(exp, "aws-eks-nodegroup-scale", "clusterName", "nodegroupName", "desiredSize")
	return err
}

var _ executor.Executor = (*LambdaThrottleExecutor)(nil)

// lambdaConcurrencyState captures whether the function had a reserved
// concurrency value before throttling: a nil pointer means it had none (account
// default), which Rollback restores by deleting the reservation we added.
type lambdaConcurrencyState struct {
	hadReservation bool
	reservedValue  int32
}

type LambdaThrottleExecutor struct {
	Logger *slog.Logger

	mu    sync.Mutex
	state map[string]lambdaConcurrencyState
}

func NewLambdaThrottleExecutor(logger *slog.Logger) *LambdaThrottleExecutor {
	return &LambdaThrottleExecutor{Logger: logger, state: make(map[string]lambdaConcurrencyState)}
}

// Execute sets reserved concurrency to 0, which makes Lambda throttle every
// invocation. The prior concurrency setting is recorded so Rollback restores it.
func (e *LambdaThrottleExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAWSParams(exp, "aws-lambda-throttle", "functionName")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	client, err := clientFactory(cfg)
	if err != nil {
		return fmt.Errorf("aws-lambda-throttle: %w", err)
	}

	fn := params["functionName"]
	cur, err := client.Lambda.GetFunctionConcurrency(ctx, &lambda.GetFunctionConcurrencyInput{
		FunctionName: &fn,
	})
	if err != nil {
		return fmt.Errorf("aws-lambda-throttle: get concurrency: %w", err)
	}

	st := lambdaConcurrencyState{}
	if cur.ReservedConcurrentExecutions != nil {
		st.hadReservation = true
		st.reservedValue = *cur.ReservedConcurrentExecutions
	}

	if _, err := client.Lambda.PutFunctionConcurrency(ctx, &lambda.PutFunctionConcurrencyInput{
		FunctionName:                 &fn,
		ReservedConcurrentExecutions: aws.Int32(0),
	}); err != nil {
		return fmt.Errorf("aws-lambda-throttle: put concurrency: %w", err)
	}

	e.mu.Lock()
	e.state[string(exp.UID)] = st
	e.mu.Unlock()

	e.Logger.Info("aws-lambda-throttle: throttled function", "region", cfg.Region, "function", fn)
	return nil
}

func (e *LambdaThrottleExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	e.mu.Lock()
	st, ok := e.state[string(exp.UID)]
	delete(e.state, string(exp.UID))
	e.mu.Unlock()
	if !ok {
		return nil
	}

	params, _ := pod.ParseParameters(exp)
	cfg := configFromParams(params)
	client, err := clientFactory(cfg)
	if err != nil {
		return fmt.Errorf("aws-lambda-throttle rollback: %w", err)
	}
	fn := params["functionName"]

	if st.hadReservation {
		if _, err := client.Lambda.PutFunctionConcurrency(ctx, &lambda.PutFunctionConcurrencyInput{
			FunctionName:                 &fn,
			ReservedConcurrentExecutions: aws.Int32(st.reservedValue),
		}); err != nil {
			return fmt.Errorf("aws-lambda-throttle rollback: restore concurrency: %w", err)
		}
	} else {
		if _, err := client.Lambda.DeleteFunctionConcurrency(ctx, &lambda.DeleteFunctionConcurrencyInput{
			FunctionName: &fn,
		}); err != nil {
			return fmt.Errorf("aws-lambda-throttle rollback: delete concurrency: %w", err)
		}
	}
	e.Logger.Info("aws-lambda-throttle: restored function concurrency", "function", fn)
	return nil
}

func (e *LambdaThrottleExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAWSParams(exp, "aws-lambda-throttle", "functionName")
	return err
}

var _ executor.Executor = (*S3BlockExecutor)(nil)

// denyAllBucketPolicy denies every S3 action on the bucket and its objects for
// all principals, simulating a loss of access to the bucket.
const denyAllBucketPolicy = `{"Version":"2012-10-17","Statement":[{"Sid":"ChaosPlaneDenyAll","Effect":"Deny","Principal":"*","Action":"s3:*","Resource":["arn:aws:s3:::%s","arn:aws:s3:::%s/*"]}]}`

// s3PolicyState records the bucket's original policy so Rollback can put it
// back; hadPolicy distinguishes "no policy existed" from "empty policy".
type s3PolicyState struct {
	hadPolicy bool
	policy    string
}

type S3BlockExecutor struct {
	Logger *slog.Logger

	mu    sync.Mutex
	state map[string]s3PolicyState
}

func NewS3BlockExecutor(logger *slog.Logger) *S3BlockExecutor {
	return &S3BlockExecutor{Logger: logger, state: make(map[string]s3PolicyState)}
}

func (e *S3BlockExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateAWSParams(exp, "aws-s3-block", "bucketName")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	client, err := clientFactory(cfg)
	if err != nil {
		return fmt.Errorf("aws-s3-block: %w", err)
	}

	bucket := params["bucketName"]
	st := s3PolicyState{}
	cur, err := client.S3.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: &bucket})
	if err != nil {
		// A bucket with no policy returns NoSuchBucketPolicy; treat that as "no
		// prior policy" rather than a hard failure so we can still apply the block.
		if !isNoSuchBucketPolicy(err) {
			return fmt.Errorf("aws-s3-block: get bucket policy: %w", err)
		}
	} else if cur.Policy != nil {
		st.hadPolicy = true
		st.policy = *cur.Policy
	}

	denyPolicy := fmt.Sprintf(denyAllBucketPolicy, bucket, bucket)
	if _, err := client.S3.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: &bucket,
		Policy: &denyPolicy,
	}); err != nil {
		return fmt.Errorf("aws-s3-block: put deny policy: %w", err)
	}

	e.mu.Lock()
	e.state[string(exp.UID)] = st
	e.mu.Unlock()

	e.Logger.Info("aws-s3-block: applied deny-all bucket policy", "region", cfg.Region, "bucket", bucket)
	return nil
}

func (e *S3BlockExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	e.mu.Lock()
	st, ok := e.state[string(exp.UID)]
	delete(e.state, string(exp.UID))
	e.mu.Unlock()
	if !ok {
		return nil
	}

	params, _ := pod.ParseParameters(exp)
	cfg := configFromParams(params)
	client, err := clientFactory(cfg)
	if err != nil {
		return fmt.Errorf("aws-s3-block rollback: %w", err)
	}
	bucket := params["bucketName"]

	if st.hadPolicy {
		if _, err := client.S3.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
			Bucket: &bucket,
			Policy: &st.policy,
		}); err != nil {
			return fmt.Errorf("aws-s3-block rollback: restore policy: %w", err)
		}
	} else {
		if _, err := client.S3.DeleteBucketPolicy(ctx, &s3.DeleteBucketPolicyInput{
			Bucket: &bucket,
		}); err != nil {
			return fmt.Errorf("aws-s3-block rollback: delete policy: %w", err)
		}
	}
	e.Logger.Info("aws-s3-block: restored bucket policy", "bucket", bucket)
	return nil
}

func (e *S3BlockExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateAWSParams(exp, "aws-s3-block", "bucketName")
	return err
}
