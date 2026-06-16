package aws

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
	"k8s.io/apimachinery/pkg/types"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeEC2 records calls and serves canned responses so AZ-isolation logic can
// be exercised without a live account.
type fakeEC2 struct {
	subnets         []ec2types.Subnet
	acls            []ec2types.NetworkAcl
	createdACLID    string
	denyEntries     int
	replaceCalls    []ec2.ReplaceNetworkAclAssociationInput
	deletedACLIDs   []string
	replaceCounter  int
	failCreateEntry bool
}

func (f *fakeEC2) StopInstances(context.Context, *ec2.StopInstancesInput, ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error) {
	return &ec2.StopInstancesOutput{}, nil
}
func (f *fakeEC2) StartInstances(context.Context, *ec2.StartInstancesInput, ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error) {
	return &ec2.StartInstancesOutput{}, nil
}
func (f *fakeEC2) TerminateInstances(context.Context, *ec2.TerminateInstancesInput, ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	return &ec2.TerminateInstancesOutput{}, nil
}
func (f *fakeEC2) DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return &ec2.DescribeSubnetsOutput{Subnets: f.subnets}, nil
}
func (f *fakeEC2) DescribeNetworkAcls(context.Context, *ec2.DescribeNetworkAclsInput, ...func(*ec2.Options)) (*ec2.DescribeNetworkAclsOutput, error) {
	return &ec2.DescribeNetworkAclsOutput{NetworkAcls: f.acls}, nil
}
func (f *fakeEC2) CreateNetworkAcl(context.Context, *ec2.CreateNetworkAclInput, ...func(*ec2.Options)) (*ec2.CreateNetworkAclOutput, error) {
	f.createdACLID = "acl-deny-123"
	return &ec2.CreateNetworkAclOutput{NetworkAcl: &ec2types.NetworkAcl{NetworkAclId: aws.String(f.createdACLID)}}, nil
}
func (f *fakeEC2) CreateNetworkAclEntry(context.Context, *ec2.CreateNetworkAclEntryInput, ...func(*ec2.Options)) (*ec2.CreateNetworkAclEntryOutput, error) {
	if f.failCreateEntry {
		return nil, fmt.Errorf("simulated entry failure")
	}
	f.denyEntries++
	return &ec2.CreateNetworkAclEntryOutput{}, nil
}
func (f *fakeEC2) ReplaceNetworkAclAssociation(_ context.Context, in *ec2.ReplaceNetworkAclAssociationInput, _ ...func(*ec2.Options)) (*ec2.ReplaceNetworkAclAssociationOutput, error) {
	f.replaceCalls = append(f.replaceCalls, *in)
	f.replaceCounter++
	return &ec2.ReplaceNetworkAclAssociationOutput{NewAssociationId: aws.String(fmt.Sprintf("aclassoc-new-%d", f.replaceCounter))}, nil
}
func (f *fakeEC2) DeleteNetworkAcl(_ context.Context, in *ec2.DeleteNetworkAclInput, _ ...func(*ec2.Options)) (*ec2.DeleteNetworkAclOutput, error) {
	f.deletedACLIDs = append(f.deletedACLIDs, aws.ToString(in.NetworkAclId))
	return &ec2.DeleteNetworkAclOutput{}, nil
}

func withFakeClient(t *testing.T, c *AWSClient) {
	t.Helper()
	orig := clientFactory
	clientFactory = func(AWSConfig) (*AWSClient, error) { return c, nil }
	t.Cleanup(func() { clientFactory = orig })
}

func TestAZFailure_Execute_IsolatesAndRollsBack(t *testing.T) {
	fe := &fakeEC2{
		subnets: []ec2types.Subnet{
			{SubnetId: aws.String("subnet-a"), VpcId: aws.String("vpc-1")},
			{SubnetId: aws.String("subnet-b"), VpcId: aws.String("vpc-1")},
		},
		acls: []ec2types.NetworkAcl{
			{
				NetworkAclId: aws.String("acl-orig-1"),
				Associations: []ec2types.NetworkAclAssociation{
					{SubnetId: aws.String("subnet-a"), NetworkAclAssociationId: aws.String("aclassoc-a")},
					{SubnetId: aws.String("subnet-b"), NetworkAclAssociationId: aws.String("aclassoc-b")},
				},
			},
		},
	}
	withFakeClient(t, &AWSClient{EC2: fe})

	e := NewAZFailureExecutor(testLogger())
	exp := makeAWSExperiment("aws-az-failure", map[string]string{"awsRegion": "us-east-1", "availabilityZone": "us-east-1a"})
	exp.UID = types.UID("uid-az")

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if fe.createdACLID == "" {
		t.Fatal("expected a deny NACL to be created")
	}
	if fe.denyEntries != 2 {
		t.Fatalf("expected 2 deny entries (ingress+egress), got %d", fe.denyEntries)
	}
	if len(fe.replaceCalls) != 2 {
		t.Fatalf("expected 2 association replacements, got %d", len(fe.replaceCalls))
	}
	for _, c := range fe.replaceCalls {
		if aws.ToString(c.NetworkAclId) != "acl-deny-123" {
			t.Fatalf("expected subnets repointed to deny acl, got %s", aws.ToString(c.NetworkAclId))
		}
	}

	fe.replaceCalls = nil
	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if len(fe.replaceCalls) != 2 {
		t.Fatalf("expected 2 restore replacements, got %d", len(fe.replaceCalls))
	}
	for _, c := range fe.replaceCalls {
		if aws.ToString(c.NetworkAclId) != "acl-orig-1" {
			t.Fatalf("expected restore to original acl, got %s", aws.ToString(c.NetworkAclId))
		}
	}
	if len(fe.deletedACLIDs) != 1 || fe.deletedACLIDs[0] != "acl-deny-123" {
		t.Fatalf("expected deny acl to be deleted on rollback, got %v", fe.deletedACLIDs)
	}
}

func TestAZFailure_Execute_NoSubnetsFails(t *testing.T) {
	withFakeClient(t, &AWSClient{EC2: &fakeEC2{}})
	e := NewAZFailureExecutor(testLogger())
	exp := makeAWSExperiment("aws-az-failure", map[string]string{"awsRegion": "us-east-1", "availabilityZone": "us-east-1a"})
	if err := e.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error when no subnets in AZ")
	}
}

func TestAZFailure_Execute_EntryFailureCleansUp(t *testing.T) {
	fe := &fakeEC2{
		subnets:         []ec2types.Subnet{{SubnetId: aws.String("subnet-a"), VpcId: aws.String("vpc-1")}},
		failCreateEntry: true,
	}
	withFakeClient(t, &AWSClient{EC2: fe})
	e := NewAZFailureExecutor(testLogger())
	exp := makeAWSExperiment("aws-az-failure", map[string]string{"awsRegion": "us-east-1", "availabilityZone": "us-east-1a"})
	if err := e.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error on entry failure")
	}
	if len(fe.deletedACLIDs) != 1 {
		t.Fatalf("expected deny acl cleaned up after failure, got %v", fe.deletedACLIDs)
	}
}

type fakeElastiCache struct {
	called *elasticache.TestFailoverInput
	err    error
}

func (f *fakeElastiCache) TestFailover(_ context.Context, in *elasticache.TestFailoverInput, _ ...func(*elasticache.Options)) (*elasticache.TestFailoverOutput, error) {
	f.called = in
	return &elasticache.TestFailoverOutput{}, f.err
}

func TestElastiCacheFailover_Execute(t *testing.T) {
	fc := &fakeElastiCache{}
	withFakeClient(t, &AWSClient{ElastiCache: fc})
	e := NewElastiCacheFailoverExecutor(testLogger())
	exp := makeAWSExperiment("aws-elasticache-failover", map[string]string{
		"awsRegion": "us-east-1", "replicationGroupId": "rg-1", "nodeGroupId": "0001",
	})
	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if fc.called == nil || aws.ToString(fc.called.ReplicationGroupId) != "rg-1" || aws.ToString(fc.called.NodeGroupId) != "0001" {
		t.Fatalf("expected TestFailover called with rg-1/0001, got %+v", fc.called)
	}
}

func TestElastiCacheFailover_Execute_Error(t *testing.T) {
	withFakeClient(t, &AWSClient{ElastiCache: &fakeElastiCache{err: fmt.Errorf("boom")}})
	e := NewElastiCacheFailoverExecutor(testLogger())
	exp := makeAWSExperiment("aws-elasticache-failover", map[string]string{
		"awsRegion": "us-east-1", "replicationGroupId": "rg-1", "nodeGroupId": "0001",
	})
	if err := e.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error from TestFailover")
	}
}

type fakeEKS struct {
	scaling     *ekstypes.NodegroupScalingConfig
	updateCalls []ekstypes.NodegroupScalingConfig
}

func (f *fakeEKS) DescribeNodegroup(context.Context, *eks.DescribeNodegroupInput, ...func(*eks.Options)) (*eks.DescribeNodegroupOutput, error) {
	return &eks.DescribeNodegroupOutput{Nodegroup: &ekstypes.Nodegroup{ScalingConfig: f.scaling}}, nil
}
func (f *fakeEKS) UpdateNodegroupConfig(_ context.Context, in *eks.UpdateNodegroupConfigInput, _ ...func(*eks.Options)) (*eks.UpdateNodegroupConfigOutput, error) {
	f.updateCalls = append(f.updateCalls, *in.ScalingConfig)
	return &eks.UpdateNodegroupConfigOutput{}, nil
}

func TestEKSNodegroupScale_ExecuteAndRollback(t *testing.T) {
	fk := &fakeEKS{scaling: &ekstypes.NodegroupScalingConfig{
		DesiredSize: aws.Int32(5), MinSize: aws.Int32(3), MaxSize: aws.Int32(10),
	}}
	withFakeClient(t, &AWSClient{EKS: fk})
	e := NewEKSNodegroupScaleExecutor(testLogger())
	exp := makeAWSExperiment("aws-eks-nodegroup-scale", map[string]string{
		"awsRegion": "us-east-1", "clusterName": "c1", "nodegroupName": "ng1", "desiredSize": "1",
	})
	exp.UID = types.UID("uid-eks")

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(fk.updateCalls) != 1 {
		t.Fatalf("expected 1 update, got %d", len(fk.updateCalls))
	}
	got := fk.updateCalls[0]
	if aws.ToInt32(got.DesiredSize) != 1 {
		t.Fatalf("expected desired 1, got %d", aws.ToInt32(got.DesiredSize))
	}
	// minSize must clamp to the new desired (1) since original min (3) exceeds it.
	if aws.ToInt32(got.MinSize) != 1 {
		t.Fatalf("expected min clamped to 1, got %d", aws.ToInt32(got.MinSize))
	}

	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if len(fk.updateCalls) != 2 {
		t.Fatalf("expected 2 updates after rollback, got %d", len(fk.updateCalls))
	}
	restored := fk.updateCalls[1]
	if aws.ToInt32(restored.DesiredSize) != 5 || aws.ToInt32(restored.MinSize) != 3 || aws.ToInt32(restored.MaxSize) != 10 {
		t.Fatalf("expected original 5/3/10 restored, got %d/%d/%d",
			aws.ToInt32(restored.DesiredSize), aws.ToInt32(restored.MinSize), aws.ToInt32(restored.MaxSize))
	}
}

type fakeLambda struct {
	reserved    *int32
	putCalls    []int32
	deleteCalls int
}

func (f *fakeLambda) GetFunctionConcurrency(context.Context, *lambda.GetFunctionConcurrencyInput, ...func(*lambda.Options)) (*lambda.GetFunctionConcurrencyOutput, error) {
	return &lambda.GetFunctionConcurrencyOutput{ReservedConcurrentExecutions: f.reserved}, nil
}
func (f *fakeLambda) PutFunctionConcurrency(_ context.Context, in *lambda.PutFunctionConcurrencyInput, _ ...func(*lambda.Options)) (*lambda.PutFunctionConcurrencyOutput, error) {
	f.putCalls = append(f.putCalls, aws.ToInt32(in.ReservedConcurrentExecutions))
	return &lambda.PutFunctionConcurrencyOutput{}, nil
}
func (f *fakeLambda) DeleteFunctionConcurrency(context.Context, *lambda.DeleteFunctionConcurrencyInput, ...func(*lambda.Options)) (*lambda.DeleteFunctionConcurrencyOutput, error) {
	f.deleteCalls++
	return &lambda.DeleteFunctionConcurrencyOutput{}, nil
}

func TestLambdaThrottle_RestoresPriorReservation(t *testing.T) {
	fl := &fakeLambda{reserved: aws.Int32(50)}
	withFakeClient(t, &AWSClient{Lambda: fl})
	e := NewLambdaThrottleExecutor(testLogger())
	exp := makeAWSExperiment("aws-lambda-throttle", map[string]string{"awsRegion": "us-east-1", "functionName": "fn"})
	exp.UID = types.UID("uid-l1")

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(fl.putCalls) != 1 || fl.putCalls[0] != 0 {
		t.Fatalf("expected concurrency set to 0, got %v", fl.putCalls)
	}
	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if len(fl.putCalls) != 2 || fl.putCalls[1] != 50 {
		t.Fatalf("expected reservation restored to 50, got %v", fl.putCalls)
	}
	if fl.deleteCalls != 0 {
		t.Fatalf("expected no delete when prior reservation existed, got %d", fl.deleteCalls)
	}
}

func TestLambdaThrottle_DeletesWhenNoPriorReservation(t *testing.T) {
	fl := &fakeLambda{reserved: nil}
	withFakeClient(t, &AWSClient{Lambda: fl})
	e := NewLambdaThrottleExecutor(testLogger())
	exp := makeAWSExperiment("aws-lambda-throttle", map[string]string{"awsRegion": "us-east-1", "functionName": "fn"})
	exp.UID = types.UID("uid-l2")

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if fl.deleteCalls != 1 {
		t.Fatalf("expected delete concurrency on rollback, got %d", fl.deleteCalls)
	}
}

type fakeS3 struct {
	policy      *string
	getErr      error
	putCalls    []string
	deleteCalls int
}

func (f *fakeS3) GetBucketPolicy(context.Context, *s3.GetBucketPolicyInput, ...func(*s3.Options)) (*s3.GetBucketPolicyOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &s3.GetBucketPolicyOutput{Policy: f.policy}, nil
}
func (f *fakeS3) PutBucketPolicy(_ context.Context, in *s3.PutBucketPolicyInput, _ ...func(*s3.Options)) (*s3.PutBucketPolicyOutput, error) {
	f.putCalls = append(f.putCalls, aws.ToString(in.Policy))
	return &s3.PutBucketPolicyOutput{}, nil
}
func (f *fakeS3) DeleteBucketPolicy(context.Context, *s3.DeleteBucketPolicyInput, ...func(*s3.Options)) (*s3.DeleteBucketPolicyOutput, error) {
	f.deleteCalls++
	return &s3.DeleteBucketPolicyOutput{}, nil
}

func TestS3Block_RestoresOriginalPolicy(t *testing.T) {
	orig := `{"Version":"2012-10-17","Statement":[]}`
	fs := &fakeS3{policy: aws.String(orig)}
	withFakeClient(t, &AWSClient{S3: fs})
	e := NewS3BlockExecutor(testLogger())
	exp := makeAWSExperiment("aws-s3-block", map[string]string{"awsRegion": "us-east-1", "bucketName": "my-bucket"})
	exp.UID = types.UID("uid-s1")

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(fs.putCalls) != 1 || !strings.Contains(fs.putCalls[0], "ChaosPlaneDenyAll") {
		t.Fatalf("expected deny-all policy applied, got %v", fs.putCalls)
	}
	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if len(fs.putCalls) != 2 || fs.putCalls[1] != orig {
		t.Fatalf("expected original policy restored, got %v", fs.putCalls)
	}
	if fs.deleteCalls != 0 {
		t.Fatalf("expected no delete when prior policy existed, got %d", fs.deleteCalls)
	}
}

func TestS3Block_DeletesWhenNoPriorPolicy(t *testing.T) {
	fs := &fakeS3{getErr: &noSuchPolicyErr{}}
	withFakeClient(t, &AWSClient{S3: fs})
	e := NewS3BlockExecutor(testLogger())
	exp := makeAWSExperiment("aws-s3-block", map[string]string{"awsRegion": "us-east-1", "bucketName": "my-bucket"})
	exp.UID = types.UID("uid-s2")

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if fs.deleteCalls != 1 {
		t.Fatalf("expected delete policy on rollback, got %d", fs.deleteCalls)
	}
}

// noSuchPolicyErr satisfies smithy.APIError with the NoSuchBucketPolicy code so
// the "bucket had no policy" branch is exercised without a live S3 error.
type noSuchPolicyErr struct{}

func (noSuchPolicyErr) Error() string                 { return "NoSuchBucketPolicy" }
func (noSuchPolicyErr) ErrorCode() string             { return "NoSuchBucketPolicy" }
func (noSuchPolicyErr) ErrorMessage() string          { return "no policy" }
func (noSuchPolicyErr) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }
