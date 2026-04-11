package azure

import (
	"testing"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func makeAzureExperiment(actionType string, params map[string]string) *v1alpha1.ChaosExperiment {
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
		ObjectMeta: metav1.ObjectMeta{Name: "test-azure", Namespace: "default"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:   v1alpha1.TargetSpec{Kind: "AzureResource"},
			Action:   v1alpha1.ActionSpec{Type: actionType, Parameters: runtime.RawExtension{Raw: []byte(raw)}},
			Duration: metav1.Duration{Duration: 60000000000},
		},
	}
}

func TestVMStopValidate(t *testing.T) {
	e := NewVMStopExecutor(nil)
	if err := e.Validate(makeAzureExperiment("azure-vm-stop", map[string]string{"subscriptionId": "sub-1", "resourceGroup": "rg-1", "vmName": "vm-1"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if err := e.Validate(makeAzureExperiment("azure-vm-stop", map[string]string{"subscriptionId": "sub-1"})); err == nil {
		t.Fatal("expected error for missing resourceGroup")
	}
}

func TestAKSScaleValidate(t *testing.T) {
	e := NewAKSNodePoolScaleExecutor(nil)
	if err := e.Validate(makeAzureExperiment("azure-aks-scale", map[string]string{"subscriptionId": "sub-1", "resourceGroup": "rg-1", "clusterName": "aks-1", "nodePool": "default", "targetCount": "3"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestCosmosDBFailoverValidate(t *testing.T) {
	e := NewCosmosDBFailoverExecutor(nil)
	if err := e.Validate(makeAzureExperiment("azure-cosmosdb-failover", map[string]string{"subscriptionId": "sub-1", "resourceGroup": "rg-1", "accountName": "cosmos-1", "targetRegion": "westus2"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}
