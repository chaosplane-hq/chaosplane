package gcp

import (
	"testing"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func makeGCPExperiment(actionType string, params map[string]string) *v1alpha1.ChaosExperiment {
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
		ObjectMeta: metav1.ObjectMeta{Name: "test-gcp", Namespace: "default"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:   v1alpha1.TargetSpec{Kind: "GCPResource"},
			Action:   v1alpha1.ActionSpec{Type: actionType, Parameters: runtime.RawExtension{Raw: []byte(raw)}},
			Duration: metav1.Duration{Duration: 60000000000},
		},
	}
}

func TestGKEScaleValidate(t *testing.T) {
	e := NewGKENodePoolScaleExecutor(nil)
	if err := e.Validate(makeGCPExperiment("gcp-gke-scale", map[string]string{"projectId": "proj-1", "clusterName": "gke-1", "nodePool": "default", "zone": "us-central1-a", "targetSize": "3"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if err := e.Validate(makeGCPExperiment("gcp-gke-scale", map[string]string{"projectId": "proj-1"})); err == nil {
		t.Fatal("expected error for missing clusterName")
	}
}

func TestCloudSQLFailoverValidate(t *testing.T) {
	e := NewCloudSQLFailoverExecutor(nil)
	if err := e.Validate(makeGCPExperiment("gcp-cloudsql-failover", map[string]string{"projectId": "proj-1", "instanceName": "sql-1"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestCloudRunStopValidate(t *testing.T) {
	e := NewCloudRunStopExecutor(nil)
	if err := e.Validate(makeGCPExperiment("gcp-cloudrun-stop", map[string]string{"projectId": "proj-1", "serviceName": "svc-1", "region": "us-central1"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}
