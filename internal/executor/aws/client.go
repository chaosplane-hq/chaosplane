package aws

import (
	"context"
	"errors"
	"fmt"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
)

// The narrow interfaces below are seams: executors depend on the subset of
// each SDK client they actually call, so unit tests can supply fakes without a
// live AWS account. The concrete *ec2.Client etc. satisfy these implicitly.

type EC2API interface {
	StopInstances(ctx context.Context, params *ec2.StopInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error)
	StartInstances(ctx context.Context, params *ec2.StartInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error)
	TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeNetworkAcls(ctx context.Context, params *ec2.DescribeNetworkAclsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkAclsOutput, error)
	CreateNetworkAcl(ctx context.Context, params *ec2.CreateNetworkAclInput, optFns ...func(*ec2.Options)) (*ec2.CreateNetworkAclOutput, error)
	CreateNetworkAclEntry(ctx context.Context, params *ec2.CreateNetworkAclEntryInput, optFns ...func(*ec2.Options)) (*ec2.CreateNetworkAclEntryOutput, error)
	ReplaceNetworkAclAssociation(ctx context.Context, params *ec2.ReplaceNetworkAclAssociationInput, optFns ...func(*ec2.Options)) (*ec2.ReplaceNetworkAclAssociationOutput, error)
	DeleteNetworkAcl(ctx context.Context, params *ec2.DeleteNetworkAclInput, optFns ...func(*ec2.Options)) (*ec2.DeleteNetworkAclOutput, error)
}

type RDSAPI interface {
	FailoverDBCluster(ctx context.Context, params *rds.FailoverDBClusterInput, optFns ...func(*rds.Options)) (*rds.FailoverDBClusterOutput, error)
}

type ECSAPI interface {
	StopTask(ctx context.Context, params *ecs.StopTaskInput, optFns ...func(*ecs.Options)) (*ecs.StopTaskOutput, error)
}

type ElastiCacheAPI interface {
	TestFailover(ctx context.Context, params *elasticache.TestFailoverInput, optFns ...func(*elasticache.Options)) (*elasticache.TestFailoverOutput, error)
}

type EKSAPI interface {
	DescribeNodegroup(ctx context.Context, params *eks.DescribeNodegroupInput, optFns ...func(*eks.Options)) (*eks.DescribeNodegroupOutput, error)
	UpdateNodegroupConfig(ctx context.Context, params *eks.UpdateNodegroupConfigInput, optFns ...func(*eks.Options)) (*eks.UpdateNodegroupConfigOutput, error)
}

type LambdaAPI interface {
	GetFunctionConcurrency(ctx context.Context, params *lambda.GetFunctionConcurrencyInput, optFns ...func(*lambda.Options)) (*lambda.GetFunctionConcurrencyOutput, error)
	PutFunctionConcurrency(ctx context.Context, params *lambda.PutFunctionConcurrencyInput, optFns ...func(*lambda.Options)) (*lambda.PutFunctionConcurrencyOutput, error)
	DeleteFunctionConcurrency(ctx context.Context, params *lambda.DeleteFunctionConcurrencyInput, optFns ...func(*lambda.Options)) (*lambda.DeleteFunctionConcurrencyOutput, error)
}

type S3API interface {
	GetBucketPolicy(ctx context.Context, params *s3.GetBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.GetBucketPolicyOutput, error)
	PutBucketPolicy(ctx context.Context, params *s3.PutBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.PutBucketPolicyOutput, error)
	DeleteBucketPolicy(ctx context.Context, params *s3.DeleteBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketPolicyOutput, error)
}

type AWSClient struct {
	EC2         EC2API
	RDS         RDSAPI
	ECS         ECSAPI
	ElastiCache ElastiCacheAPI
	EKS         EKSAPI
	Lambda      LambdaAPI
	S3          S3API
}

// clientFactory is a swappable seam so tests inject fakes; a package var keeps
// constructor signatures identical to the other cloud executors.
var clientFactory = NewAWSClient

func NewAWSClient(cfg AWSConfig) (*AWSClient, error) {
	opts := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(cfg.Region),
	}

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken),
		))
	}

	sdkCfg, err := awscfg.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &AWSClient{
		EC2:         ec2.NewFromConfig(sdkCfg),
		RDS:         rds.NewFromConfig(sdkCfg),
		ECS:         ecs.NewFromConfig(sdkCfg),
		ElastiCache: elasticache.NewFromConfig(sdkCfg),
		EKS:         eks.NewFromConfig(sdkCfg),
		Lambda:      lambda.NewFromConfig(sdkCfg),
		S3:          s3.NewFromConfig(sdkCfg),
	}, nil
}

// isNoSuchBucketPolicy reports whether err is S3's NoSuchBucketPolicy, which is
// returned (not as a typed error) when a bucket simply has no policy set.
func isNoSuchBucketPolicy(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "NoSuchBucketPolicy"
	}
	return false
}
