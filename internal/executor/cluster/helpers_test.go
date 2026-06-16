package cluster_test

import (
	"encoding/json"
	"io"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

var testScheme = runtime.NewScheme()

func init() {
	_ = corev1.AddToScheme(testScheme)
	_ = v1alpha1.AddToScheme(testScheme)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newExp(action, namespace, name string, params map[string]string) *v1alpha1.ChaosExperiment {
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-exp",
			Namespace: "default",
			UID:       types.UID("cluster-uid-1"),
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Action: v1alpha1.ActionSpec{Type: action},
			Target: v1alpha1.TargetSpec{
				Kind:      "K8sResource",
				Namespace: namespace,
				Names:     []string{name},
			},
		},
	}
	if params != nil {
		raw, _ := json.Marshal(params)
		exp.Spec.Action.Parameters = runtime.RawExtension{Raw: raw}
	}
	return exp
}

func newExpNoNames(action string, params map[string]string) *v1alpha1.ChaosExperiment {
	exp := newExp(action, "", "", params)
	exp.Spec.Target.Names = nil
	return exp
}
