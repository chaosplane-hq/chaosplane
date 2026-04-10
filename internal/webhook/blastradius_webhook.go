package webhook

import (
	"context"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type BlastRadiusValidator struct{}

func (v *BlastRadiusValidator) Handle(_ context.Context, _ admission.Request) admission.Response {
	return admission.Allowed("policy evaluation not yet implemented")
}

var _ admission.Handler = (*BlastRadiusValidator)(nil)

func NewBlastRadiusWebhook() *admission.Webhook {
	return &admission.Webhook{
		Handler: &BlastRadiusValidator{},
	}
}

func NewBlastRadiusHealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
