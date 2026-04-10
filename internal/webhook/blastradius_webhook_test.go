package webhook_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/webhook"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	admission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	return s
}

func makeExperiment(ns, kind, actionType string, names []string, duration time.Duration, labelSelector *metav1.LabelSelector) v1alpha1.ChaosExperiment {
	exp := v1alpha1.ChaosExperiment{
		TypeMeta: metav1.TypeMeta{APIVersion: "chaos.chaosplane.io/v1alpha1", Kind: "ChaosExperiment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-experiment",
			Namespace: ns,
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Kind:          kind,
				Namespace:     ns,
				Names:         names,
				LabelSelector: labelSelector,
			},
			Action: v1alpha1.ActionSpec{
				Type:     actionType,
				Duration: &metav1.Duration{Duration: duration},
			},
			Duration: metav1.Duration{Duration: duration},
		},
	}
	return exp
}

func makeRequest(exp v1alpha1.ChaosExperiment) admission.Request {
	raw, _ := json.Marshal(exp)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "test-uid",
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: raw},
			Resource:  metav1.GroupVersionResource{Group: "chaos.chaosplane.io", Version: "v1alpha1", Resource: "chaosexperiments"},
		},
	}
}

func int32Ptr(i int32) *int32 { return &i }

func TestBlastRadiusWebhook_NoPolicies(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"nginx"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if !resp.Allowed {
		t.Fatalf("expected allowed with no policies, got denied: %v", resp.Result)
	}
}

func TestBlastRadiusWebhook_DisabledEnforcement(t *testing.T) {
	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "disabled-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement: v1alpha1.EnforcementMode("Disabled"),
			Scope:       v1alpha1.ScopeSpec{},
			TargetLimits: v1alpha1.TargetLimitsSpec{
				MaxTargets: int32Ptr(1),
			},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{
				Namespaces: []string{"kube-system"},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("kube-system", "Pod", "pod-kill", []string{"a", "b", "c"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if !resp.Allowed {
		t.Fatalf("expected allowed with disabled enforcement, got denied: %v", resp.Result)
	}
}

func TestBlastRadiusWebhook_ProtectedNamespace(t *testing.T) {
	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:  v1alpha1.EnforcementEnforce,
			Scope:        v1alpha1.ScopeSpec{},
			TargetLimits: v1alpha1.TargetLimitsSpec{},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{
				Namespaces: []string{"kube-system", "chaosplane-system"},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("kube-system", "Pod", "pod-kill", []string{"coredns"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if resp.Allowed {
		t.Fatal("expected denied for protected namespace kube-system")
	}
	if resp.Result == nil || resp.Result.Message == "" {
		t.Fatal("expected denial message")
	}
}

func TestBlastRadiusWebhook_ProtectedLabel(t *testing.T) {
	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "label-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:  v1alpha1.EnforcementEnforce,
			Scope:        v1alpha1.ScopeSpec{},
			TargetLimits: v1alpha1.TargetLimitsSpec{},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{
				Labels: map[string]string{"critical": "true"},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("default", "Pod", "pod-kill", nil, 2*time.Minute,
		&metav1.LabelSelector{MatchLabels: map[string]string{"critical": "true"}})
	resp := v.Handle(context.Background(), makeRequest(exp))
	if resp.Allowed {
		t.Fatal("expected denied for protected label critical=true")
	}
}

func TestBlastRadiusWebhook_ProtectedResourceName(t *testing.T) {
	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "name-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:  v1alpha1.EnforcementEnforce,
			Scope:        v1alpha1.ScopeSpec{},
			TargetLimits: v1alpha1.TargetLimitsSpec{},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{
				Names: []v1alpha1.ProtectedResource{
					{Kind: "Pod", Name: "etcd-master", Namespace: "kube-system"},
				},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("kube-system", "Pod", "pod-kill", []string{"etcd-master"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if resp.Allowed {
		t.Fatal("expected denied for protected resource etcd-master")
	}
}

func TestBlastRadiusWebhook_ActionNotAllowed(t *testing.T) {
	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "action-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:        v1alpha1.EnforcementEnforce,
			Scope:              v1alpha1.ScopeSpec{},
			TargetLimits:       v1alpha1.TargetLimitsSpec{},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
			ActionLimits: &v1alpha1.ActionLimitsSpec{
				AllowedActions: []string{"pod-kill", "network-delay"},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("default", "Pod", "node-drain", []string{"worker-1"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if resp.Allowed {
		t.Fatal("expected denied for disallowed action node-drain")
	}
}

func TestBlastRadiusWebhook_DurationExceedsMax(t *testing.T) {
	maxDur := metav1.Duration{Duration: 5 * time.Minute}
	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "duration-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:        v1alpha1.EnforcementEnforce,
			Scope:              v1alpha1.ScopeSpec{},
			TargetLimits:       v1alpha1.TargetLimitsSpec{},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
			ActionLimits: &v1alpha1.ActionLimitsSpec{
				MaxDuration: &maxDur,
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"nginx"}, 10*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if resp.Allowed {
		t.Fatal("expected denied for duration exceeding maxDuration")
	}
}

func TestBlastRadiusWebhook_TargetCountExceedsMax(t *testing.T) {
	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "target-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement: v1alpha1.EnforcementEnforce,
			Scope:       v1alpha1.ScopeSpec{},
			TargetLimits: v1alpha1.TargetLimitsSpec{
				MaxTargets: int32Ptr(2),
			},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"a", "b", "c", "d"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if resp.Allowed {
		t.Fatal("expected denied for target count exceeding maxTargets")
	}
}

func TestBlastRadiusWebhook_AuditMode(t *testing.T) {
	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "audit-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement: v1alpha1.EnforcementAudit,
			Scope:       v1alpha1.ScopeSpec{},
			TargetLimits: v1alpha1.TargetLimitsSpec{
				MaxTargets: int32Ptr(1),
			},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"a", "b", "c"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if !resp.Allowed {
		t.Fatal("expected allowed in audit mode")
	}
	if resp.AuditAnnotations == nil {
		t.Fatal("expected audit annotations")
	}
	if _, ok := resp.AuditAnnotations["chaosplane.io/audit-warnings"]; !ok {
		t.Fatal("expected chaosplane.io/audit-warnings annotation")
	}
}

func TestBlastRadiusWebhook_MultiplePolicies_DenyWins(t *testing.T) {
	allowPolicy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:        v1alpha1.EnforcementEnforce,
			Scope:              v1alpha1.ScopeSpec{},
			TargetLimits:       v1alpha1.TargetLimitsSpec{MaxTargets: int32Ptr(100)},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
		},
	}
	denyPolicy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "deny-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:        v1alpha1.EnforcementEnforce,
			Scope:              v1alpha1.ScopeSpec{},
			TargetLimits:       v1alpha1.TargetLimitsSpec{MaxTargets: int32Ptr(1)},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(allowPolicy, denyPolicy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"a", "b", "c"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if resp.Allowed {
		t.Fatal("expected denied when any policy denies")
	}
}

func TestBlastRadiusWebhook_ScopeNamespaceFiltering(t *testing.T) {
	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "scoped-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement: v1alpha1.EnforcementEnforce,
			Scope: v1alpha1.ScopeSpec{
				Namespaces: []string{"production", "prod-*"},
			},
			TargetLimits:       v1alpha1.TargetLimitsSpec{MaxTargets: int32Ptr(1)},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	// "staging" doesn't match scope → policy doesn't apply → allowed
	exp := makeExperiment("staging", "Pod", "pod-kill", []string{"a", "b", "c"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if !resp.Allowed {
		t.Fatal("expected allowed for namespace outside policy scope")
	}

	// "prod-us" matches "prod-*" → policy applies → denied (too many targets)
	exp2 := makeExperiment("prod-us", "Pod", "pod-kill", []string{"a", "b", "c"}, 2*time.Minute, nil)
	resp2 := v.Handle(context.Background(), makeRequest(exp2))
	if resp2.Allowed {
		t.Fatal("expected denied for namespace matching policy scope prod-*")
	}
}

func TestBlastRadiusWebhook_AllowedExperiment(t *testing.T) {
	maxDur := metav1.Duration{Duration: 10 * time.Minute}
	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "strict-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement: v1alpha1.EnforcementEnforce,
			Scope:       v1alpha1.ScopeSpec{},
			TargetLimits: v1alpha1.TargetLimitsSpec{
				MaxTargets: int32Ptr(5),
			},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{
				Namespaces: []string{"kube-system"},
			},
			ActionLimits: &v1alpha1.ActionLimitsSpec{
				AllowedActions: []string{"pod-kill", "network-delay"},
				MaxDuration:    &maxDur,
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"nginx"}, 5*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if !resp.Allowed {
		t.Fatalf("expected allowed for compliant experiment, got denied: %v", resp.Result)
	}
}

func TestBlastRadiusWebhook_NoClient(t *testing.T) {
	v := &webhook.BlastRadiusValidator{}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"nginx"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if !resp.Allowed {
		t.Fatalf("expected allowed with no client, got denied: %v", resp.Result)
	}
}

func TestBlastRadiusWebhook_TimeWindows_EmptyAlwaysAllowed(t *testing.T) {
	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "no-tw-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:        v1alpha1.EnforcementEnforce,
			Scope:              v1alpha1.ScopeSpec{},
			TargetLimits:       v1alpha1.TargetLimitsSpec{},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{Client: cl}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"nginx"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if !resp.Allowed {
		t.Fatalf("expected allowed with no time windows, got denied: %v", resp.Result)
	}
}

func TestBlastRadiusWebhook_TimeWindows_BlockedDenies(t *testing.T) {
	// Wednesday 10:30 UTC — blocked window is weekdays 9-17 UTC
	fixedNow := time.Date(2025, 6, 11, 10, 30, 0, 0, time.UTC)

	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "blocked-tw-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:        v1alpha1.EnforcementEnforce,
			Scope:              v1alpha1.ScopeSpec{},
			TargetLimits:       v1alpha1.TargetLimitsSpec{},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
			TimeWindows: &v1alpha1.TimeWindowsSpec{
				Blocked: []v1alpha1.TimeWindow{
					{Name: "business-hours", Schedule: "0 9 * * 1-5", Duration: "8h", Timezone: "UTC"},
				},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{
		Client:  cl,
		NowFunc: func() time.Time { return fixedNow },
	}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"nginx"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if resp.Allowed {
		t.Fatal("expected denied during blocked time window")
	}
	if resp.Result == nil || !strings.Contains(resp.Result.Message, "blocked by time window") {
		t.Fatalf("expected blocked time window message, got: %v", resp.Result)
	}
}

func TestBlastRadiusWebhook_TimeWindows_AllowedPermits(t *testing.T) {
	// Wednesday 10:30 UTC — allowed window is weekdays 9-17 UTC
	fixedNow := time.Date(2025, 6, 11, 10, 30, 0, 0, time.UTC)

	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allowed-tw-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:        v1alpha1.EnforcementEnforce,
			Scope:              v1alpha1.ScopeSpec{},
			TargetLimits:       v1alpha1.TargetLimitsSpec{},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
			TimeWindows: &v1alpha1.TimeWindowsSpec{
				Allowed: []v1alpha1.TimeWindow{
					{Name: "business-hours", Schedule: "0 9 * * 1-5", Duration: "8h", Timezone: "UTC"},
				},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{
		Client:  cl,
		NowFunc: func() time.Time { return fixedNow },
	}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"nginx"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if !resp.Allowed {
		t.Fatalf("expected allowed during allowed time window, got denied: %v", resp.Result)
	}
}

func TestBlastRadiusWebhook_TimeWindows_AllowedDeniesOutside(t *testing.T) {
	// Saturday 10:30 UTC — allowed window is weekdays only
	fixedNow := time.Date(2025, 6, 14, 10, 30, 0, 0, time.UTC)

	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allowed-tw-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:        v1alpha1.EnforcementEnforce,
			Scope:              v1alpha1.ScopeSpec{},
			TargetLimits:       v1alpha1.TargetLimitsSpec{},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
			TimeWindows: &v1alpha1.TimeWindowsSpec{
				Allowed: []v1alpha1.TimeWindow{
					{Name: "business-hours", Schedule: "0 9 * * 1-5", Duration: "8h", Timezone: "UTC"},
				},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{
		Client:  cl,
		NowFunc: func() time.Time { return fixedNow },
	}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"nginx"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if resp.Allowed {
		t.Fatal("expected denied outside allowed time window (Saturday)")
	}
	if resp.Result == nil || !strings.Contains(resp.Result.Message, "outside allowed time window") {
		t.Fatalf("expected outside allowed time window message, got: %v", resp.Result)
	}
}

func TestBlastRadiusWebhook_TimeWindows_BlockedWinsOverAllowed(t *testing.T) {
	// Wednesday 10:30 UTC — both allowed and blocked match
	fixedNow := time.Date(2025, 6, 11, 10, 30, 0, 0, time.UTC)

	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "conflict-tw-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:        v1alpha1.EnforcementEnforce,
			Scope:              v1alpha1.ScopeSpec{},
			TargetLimits:       v1alpha1.TargetLimitsSpec{},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
			TimeWindows: &v1alpha1.TimeWindowsSpec{
				Allowed: []v1alpha1.TimeWindow{
					{Name: "business-hours", Schedule: "0 9 * * 1-5", Duration: "8h", Timezone: "UTC"},
				},
				Blocked: []v1alpha1.TimeWindow{
					{Name: "maintenance", Schedule: "0 10 * * 3", Duration: "2h", Timezone: "UTC"},
				},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{
		Client:  cl,
		NowFunc: func() time.Time { return fixedNow },
	}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"nginx"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if resp.Allowed {
		t.Fatal("expected denied: blocked should win over allowed")
	}
	if resp.Result == nil || !strings.Contains(resp.Result.Message, "blocked by time window") {
		t.Fatalf("expected blocked message, got: %v", resp.Result)
	}
}

func TestBlastRadiusWebhook_TimeWindows_TimezoneHandling(t *testing.T) {
	// 2025-06-11 10:30 UTC = 2025-06-11 19:30 KST
	// Allowed window: weekdays 18:00 KST for 4h → 18:00-22:00 KST
	fixedNow := time.Date(2025, 6, 11, 10, 30, 0, 0, time.UTC)

	policy := &v1alpha1.BlastRadiusPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "tz-policy"},
		Spec: v1alpha1.BlastRadiusPolicySpec{
			Enforcement:        v1alpha1.EnforcementEnforce,
			Scope:              v1alpha1.ScopeSpec{},
			TargetLimits:       v1alpha1.TargetLimitsSpec{},
			ProtectedResources: v1alpha1.ProtectedResourcesSpec{},
			TimeWindows: &v1alpha1.TimeWindowsSpec{
				Allowed: []v1alpha1.TimeWindow{
					{Name: "evening-kst", Schedule: "0 18 * * 1-5", Duration: "4h", Timezone: "Asia/Seoul"},
				},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(policy).Build()
	v := &webhook.BlastRadiusValidator{
		Client:  cl,
		NowFunc: func() time.Time { return fixedNow },
	}

	exp := makeExperiment("default", "Pod", "pod-kill", []string{"nginx"}, 2*time.Minute, nil)
	resp := v.Handle(context.Background(), makeRequest(exp))
	if !resp.Allowed {
		t.Fatalf("expected allowed: 19:30 KST is within 18:00-22:00 KST window, got denied: %v", resp.Result)
	}

	// 2025-06-11 08:00 UTC = 2025-06-11 17:00 KST — outside 18:00-22:00 KST
	earlyNow := time.Date(2025, 6, 11, 8, 0, 0, 0, time.UTC)
	v2 := &webhook.BlastRadiusValidator{
		Client:  cl,
		NowFunc: func() time.Time { return earlyNow },
	}
	resp2 := v2.Handle(context.Background(), makeRequest(exp))
	if resp2.Allowed {
		t.Fatal("expected denied: 17:00 KST is outside 18:00-22:00 KST window")
	}
}
