package webhook_test

import (
	"context"
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/chaosplane-hq/chaosplane/internal/webhook"
)

func TestBlastRadiusValidatorAlwaysAllows(t *testing.T) {
	v := &webhook.BlastRadiusValidator{}

	raw, _ := json.Marshal(map[string]string{"foo": "bar"})

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "test-uid",
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: raw},
			Resource:  metav1.GroupVersionResource{Group: "chaos.chaosplane.io", Version: "v1alpha1", Resource: "chaosexperiments"},
		},
	}

	resp := v.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %s", resp.Result.Message)
	}
}
