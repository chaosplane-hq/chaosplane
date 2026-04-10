package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type BlastRadiusValidator struct {
	Client client.Client
	Logger *slog.Logger
}

var _ admission.Handler = (*BlastRadiusValidator)(nil)

func (v *BlastRadiusValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := v.logger()

	experiment := &v1alpha1.ChaosExperiment{}
	if err := json.Unmarshal(req.Object.Raw, experiment); err != nil {
		logger.Error("failed to decode ChaosExperiment", "error", err)
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode ChaosExperiment: %w", err))
	}

	if v.Client == nil {
		return admission.Allowed("no policy client configured")
	}

	policyList := &v1alpha1.BlastRadiusPolicyList{}
	if err := v.Client.List(ctx, policyList); err != nil {
		logger.Error("failed to list BlastRadiusPolicies", "error", err)
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to list policies: %w", err))
	}

	if len(policyList.Items) == 0 {
		return admission.Allowed("no blast radius policies configured")
	}

	var auditWarnings []string

	for i := range policyList.Items {
		policy := &policyList.Items[i]

		if !policyMatchesExperiment(policy, experiment) {
			continue
		}

		result := evaluatePolicy(policy, experiment)
		if result.allowed {
			continue
		}

		if policy.Spec.Enforcement == v1alpha1.EnforcementAudit {
			warning := fmt.Sprintf("policy %q would deny: %s", policy.Name, result.reason)
			logger.Warn("audit mode violation", "policy", policy.Name, "experiment", experiment.Name, "reason", result.reason)
			auditWarnings = append(auditWarnings, warning)
			continue
		}

		logger.Info("policy denied experiment", "policy", policy.Name, "experiment", experiment.Name, "reason", result.reason)
		return admission.Denied(fmt.Sprintf("denied by policy %q: %s", policy.Name, result.reason))
	}

	resp := admission.Allowed("all policies passed")
	if len(auditWarnings) > 0 {
		warningsJSON, _ := json.Marshal(auditWarnings)
		resp.AuditAnnotations = map[string]string{
			"chaosplane.io/audit-warnings": string(warningsJSON),
		}
	}
	return resp
}

type evalResult struct {
	allowed bool
	reason  string
}

func evaluatePolicy(policy *v1alpha1.BlastRadiusPolicy, exp *v1alpha1.ChaosExperiment) evalResult {
	// Step 1: unknown/disabled enforcement → allow
	if policy.Spec.Enforcement != v1alpha1.EnforcementEnforce && policy.Spec.Enforcement != v1alpha1.EnforcementAudit {
		return evalResult{allowed: true}
	}

	// Step 2: protected resources
	if reason := checkProtectedResources(policy, exp); reason != "" {
		return evalResult{allowed: false, reason: reason}
	}

	// Step 3: protected namespaces
	if reason := checkProtectedNamespaces(policy, exp); reason != "" {
		return evalResult{allowed: false, reason: reason}
	}

	// Step 4: action limits
	if reason := checkActionLimits(policy, exp); reason != "" {
		return evalResult{allowed: false, reason: reason}
	}

	// Step 5: target limits
	if reason := checkTargetLimits(policy, exp); reason != "" {
		return evalResult{allowed: false, reason: reason}
	}

	// Step 6: all checks passed
	return evalResult{allowed: true}
}

func policyMatchesExperiment(policy *v1alpha1.BlastRadiusPolicy, exp *v1alpha1.ChaosExperiment) bool {
	scope := policy.Spec.Scope

	if len(scope.Namespaces) > 0 {
		matched := false
		for _, ns := range scope.Namespaces {
			if matchGlob(ns, exp.Spec.Target.Namespace) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if scope.LabelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(scope.LabelSelector)
		if err != nil {
			return false
		}
		if !selector.Matches(labels.Set(exp.Labels)) {
			return false
		}
	}

	return true
}

func checkProtectedResources(policy *v1alpha1.BlastRadiusPolicy, exp *v1alpha1.ChaosExperiment) string {
	protected := policy.Spec.ProtectedResources

	for _, pr := range protected.Names {
		if pr.Kind != "" && !strings.EqualFold(pr.Kind, exp.Spec.Target.Kind) {
			continue
		}
		if pr.Namespace != "" && pr.Namespace != exp.Spec.Target.Namespace {
			continue
		}
		for _, targetName := range exp.Spec.Target.Names {
			if pr.Name == targetName {
				return fmt.Sprintf("targets protected resource %s/%s", pr.Kind, pr.Name)
			}
		}
	}

	if len(protected.Labels) > 0 && exp.Spec.Target.LabelSelector != nil {
		if exp.Spec.Target.LabelSelector.MatchLabels != nil {
			for k, v := range protected.Labels {
				if expVal, ok := exp.Spec.Target.LabelSelector.MatchLabels[k]; ok && expVal == v {
					return fmt.Sprintf("targets resources with protected label %s=%s", k, v)
				}
			}
		}
	}

	return ""
}

func checkProtectedNamespaces(policy *v1alpha1.BlastRadiusPolicy, exp *v1alpha1.ChaosExperiment) string {
	for _, ns := range policy.Spec.ProtectedResources.Namespaces {
		if matchGlob(ns, exp.Spec.Target.Namespace) {
			return fmt.Sprintf("namespace %q is protected", exp.Spec.Target.Namespace)
		}
	}
	return ""
}

func checkActionLimits(policy *v1alpha1.BlastRadiusPolicy, exp *v1alpha1.ChaosExperiment) string {
	limits := policy.Spec.ActionLimits
	if limits == nil {
		return ""
	}

	if len(limits.AllowedActions) > 0 {
		found := false
		for _, a := range limits.AllowedActions {
			if a == exp.Spec.Action.Type {
				found = true
				break
			}
		}
		if !found {
			return fmt.Sprintf("action %q is not in allowed actions %v", exp.Spec.Action.Type, limits.AllowedActions)
		}
	}

	if limits.MaxDuration != nil {
		expDuration := getExperimentDuration(exp)
		if expDuration > limits.MaxDuration.Duration {
			return fmt.Sprintf("duration %s exceeds max allowed %s", expDuration, limits.MaxDuration.Duration)
		}
	}

	return ""
}

func checkTargetLimits(policy *v1alpha1.BlastRadiusPolicy, exp *v1alpha1.ChaosExperiment) string {
	limits := policy.Spec.TargetLimits

	if limits.MaxTargets != nil && len(exp.Spec.Target.Names) > 0 {
		if int32(len(exp.Spec.Target.Names)) > *limits.MaxTargets {
			return fmt.Sprintf("target count %d exceeds maxTargets %d", len(exp.Spec.Target.Names), *limits.MaxTargets)
		}
	}

	if limits.MaxPercentage != nil && exp.Spec.Execution.Parallelism != nil {
		if *exp.Spec.Execution.Parallelism > *limits.MaxPercentage {
			return fmt.Sprintf("parallelism %d exceeds maxPercentage %d", *exp.Spec.Execution.Parallelism, *limits.MaxPercentage)
		}
	}

	return ""
}

func getExperimentDuration(exp *v1alpha1.ChaosExperiment) time.Duration {
	if exp.Spec.Action.Duration != nil {
		return exp.Spec.Action.Duration.Duration
	}
	return exp.Spec.Duration.Duration
}

func matchGlob(pattern, value string) bool {
	if pattern == value {
		return true
	}
	matched, err := filepath.Match(pattern, value)
	if err != nil {
		return false
	}
	return matched
}

func (v *BlastRadiusValidator) logger() *slog.Logger {
	if v.Logger != nil {
		return v.Logger
	}
	return slog.Default()
}

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
