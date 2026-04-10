package aws

import (
	"testing"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func makeAWSExperiment(actionType string, params map[string]string) *v1alpha1.ChaosExperiment {
	raw := "{"
	first := true
	for k, v := range params {
		if !first {
			raw += ","
		}
		raw += `"` + k + `":"` + v + `"`
		first = false
	}
	raw += "}"

	return &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-aws", Namespace: "default"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{Kind: "AWSResource"},
			Action: v1alpha1.ActionSpec{
				Type:       actionType,
				Parameters: runtime.RawExtension{Raw: []byte(raw)},
			},
			Duration: metav1.Duration{Duration: 60000000000},
		},
	}
}

func TestEC2StopValidate(t *testing.T) {
	e := NewEC2StopExecutor(nil)
	if err := e.Validate(makeAWSExperiment("aws-ec2-stop", map[string]string{"awsRegion": "us-east-1", "instanceIds": "i-123"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if err := e.Validate(makeAWSExperiment("aws-ec2-stop", map[string]string{"instanceIds": "i-123"})); err == nil {
		t.Fatal("expected error for missing awsRegion")
	}
	if err := e.Validate(makeAWSExperiment("aws-ec2-stop", map[string]string{"awsRegion": "us-east-1"})); err == nil {
		t.Fatal("expected error for missing instanceIds")
	}
}

func TestRDSFailoverValidate(t *testing.T) {
	e := NewRDSFailoverExecutor(nil)
	if err := e.Validate(makeAWSExperiment("aws-rds-failover", map[string]string{"awsRegion": "us-west-2", "dbClusterIdentifier": "my-cluster"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if err := e.Validate(makeAWSExperiment("aws-rds-failover", map[string]string{"awsRegion": "us-west-2"})); err == nil {
		t.Fatal("expected error for missing dbClusterIdentifier")
	}
}

func TestECSStopTaskValidate(t *testing.T) {
	e := NewECSStopTaskExecutor(nil)
	if err := e.Validate(makeAWSExperiment("aws-ecs-stop-task", map[string]string{"awsRegion": "eu-west-1", "cluster": "my-cluster", "taskArn": "arn:aws:ecs:..."})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestAZFailureValidate(t *testing.T) {
	e := NewAZFailureExecutor(nil)
	if err := e.Validate(makeAWSExperiment("aws-az-failure", map[string]string{"awsRegion": "ap-northeast-2", "availabilityZone": "ap-northeast-2a"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}
