package vm

import (
	"testing"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func makeVMExperiment(actionType string, params map[string]string) *v1alpha1.ChaosExperiment {
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
		ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{Kind: "VM"},
			Action: v1alpha1.ActionSpec{
				Type:       actionType,
				Parameters: runtime.RawExtension{Raw: []byte(raw)},
			},
			Duration: metav1.Duration{Duration: 60000000000},
		},
	}
}

func TestCPUStressValidate(t *testing.T) {
	e := NewCPUStressExecutor(nil)
	if err := e.Validate(makeVMExperiment("vm-cpu-stress", map[string]string{"sshHost": "10.0.0.1"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if err := e.Validate(makeVMExperiment("vm-cpu-stress", map[string]string{})); err == nil {
		t.Fatal("expected error for missing sshHost")
	}
}

func TestNetworkDelayValidate(t *testing.T) {
	e := NewNetworkDelayExecutor(nil)
	if err := e.Validate(makeVMExperiment("vm-network-delay", map[string]string{"sshHost": "10.0.0.1", "latency": "100ms", "iface": "eth0"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if err := e.Validate(makeVMExperiment("vm-network-delay", map[string]string{"sshHost": "10.0.0.1"})); err == nil {
		t.Fatal("expected error for missing latency")
	}
}

func TestProcessKillValidate(t *testing.T) {
	e := NewProcessKillExecutor(nil)
	if err := e.Validate(makeVMExperiment("vm-process-kill", map[string]string{"sshHost": "10.0.0.1", "processName": "nginx"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestProcessSuspendValidate(t *testing.T) {
	e := NewProcessSuspendExecutor(nil)
	if err := e.Validate(makeVMExperiment("vm-process-suspend", map[string]string{"sshHost": "10.0.0.1", "processName": "redis"})); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}
