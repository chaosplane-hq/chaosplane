package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type BlastRadiusValidator struct {
	Client  client.Client
	Logger  *slog.Logger
	NowFunc func() time.Time
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

		result := evaluatePolicyAt(policy, experiment, v.now())
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

func evaluatePolicyAt(policy *v1alpha1.BlastRadiusPolicy, exp *v1alpha1.ChaosExperiment, now time.Time) evalResult {
	if policy.Spec.Enforcement != v1alpha1.EnforcementEnforce && policy.Spec.Enforcement != v1alpha1.EnforcementAudit {
		return evalResult{allowed: true}
	}

	if reason := checkProtectedResources(policy, exp); reason != "" {
		return evalResult{allowed: false, reason: reason}
	}

	if reason := checkProtectedNamespaces(policy, exp); reason != "" {
		return evalResult{allowed: false, reason: reason}
	}

	if reason := checkActionLimits(policy, exp); reason != "" {
		return evalResult{allowed: false, reason: reason}
	}

	if reason := checkTimeWindows(policy, now); reason != "" {
		return evalResult{allowed: false, reason: reason}
	}

	if reason := checkTargetLimits(policy, exp); reason != "" {
		return evalResult{allowed: false, reason: reason}
	}

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

func (v *BlastRadiusValidator) now() time.Time {
	if v.NowFunc != nil {
		return v.NowFunc()
	}
	return time.Now()
}

func checkTimeWindows(policy *v1alpha1.BlastRadiusPolicy, now time.Time) string {
	tw := policy.Spec.TimeWindows
	if tw == nil {
		return ""
	}

	for _, b := range tw.Blocked {
		if isInTimeWindow(b, now) {
			return fmt.Sprintf("blocked by time window %q", b.Name)
		}
	}

	if len(tw.Allowed) == 0 {
		return ""
	}

	for _, a := range tw.Allowed {
		if isInTimeWindow(a, now) {
			return ""
		}
	}
	return "outside allowed time window"
}

func isInTimeWindow(tw v1alpha1.TimeWindow, now time.Time) bool {
	loc, err := time.LoadLocation(tw.Timezone)
	if err != nil {
		return false
	}
	now = now.In(loc)

	dur, err := time.ParseDuration(tw.Duration)
	if err != nil {
		return false
	}

	maxMinutes := int(dur.Minutes())
	if maxMinutes < 1 {
		maxMinutes = 1
	}

	for i := 0; i <= maxMinutes; i++ {
		candidate := now.Add(-time.Duration(i) * time.Minute).Truncate(time.Minute)
		if cronMatches(tw.Schedule, candidate) {
			return true
		}
	}
	return false
}

func cronMatches(schedule string, t time.Time) bool {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return false
	}
	return fieldMatches(fields[0], t.Minute(), 0, 59) &&
		fieldMatches(fields[1], t.Hour(), 0, 23) &&
		fieldMatches(fields[2], t.Day(), 1, 31) &&
		fieldMatches(fields[3], int(t.Month()), 1, 12) &&
		fieldMatches(fields[4], int(t.Weekday()), 0, 6)
}

func fieldMatches(field string, value, min, max int) bool {
	if field == "*" {
		return true
	}
	for _, part := range strings.Split(field, ",") {
		if matchCronPart(part, value, min, max) {
			return true
		}
	}
	return false
}

func matchCronPart(part string, value, min, max int) bool {
	if strings.Contains(part, "/") {
		tokens := strings.SplitN(part, "/", 2)
		step, err := strconv.Atoi(tokens[1])
		if err != nil || step <= 0 {
			return false
		}
		base := tokens[0]
		start := min
		if base != "*" {
			s, err := strconv.Atoi(base)
			if err != nil {
				return false
			}
			start = s
		}
		if value < start {
			return false
		}
		return (value-start)%step == 0
	}

	if strings.Contains(part, "-") {
		bounds := strings.SplitN(part, "-", 2)
		lo, err1 := strconv.Atoi(bounds[0])
		hi, err2 := strconv.Atoi(bounds[1])
		if err1 != nil || err2 != nil {
			return false
		}
		return value >= lo && value <= hi
	}

	n, err := strconv.Atoi(part)
	if err != nil {
		return false
	}
	return value == n
}
