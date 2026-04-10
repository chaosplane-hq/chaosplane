package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Enforcement",type=string,JSONPath=`.spec.enforcement`
// +kubebuilder:printcolumn:name="Max Targets",type=integer,JSONPath=`.spec.targetLimits.maxTargets`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type BlastRadiusPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec BlastRadiusPolicySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type BlastRadiusPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BlastRadiusPolicy `json:"items"`
}

type BlastRadiusPolicySpec struct {
	Scope              ScopeSpec              `json:"scope"`
	TargetLimits       TargetLimitsSpec       `json:"targetLimits"`
	ProtectedResources ProtectedResourcesSpec `json:"protectedResources"`
	ActionLimits       *ActionLimitsSpec      `json:"actionLimits,omitempty"`
	TimeWindows        *TimeWindowsSpec       `json:"timeWindows,omitempty"`
	Enforcement        EnforcementMode        `json:"enforcement"`
}

type TimeWindowsSpec struct {
	Allowed []TimeWindow `json:"allowed,omitempty"`
	Blocked []TimeWindow `json:"blocked,omitempty"`
}

type TimeWindow struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"` // 5-field cron expression (e.g. "0 9 * * 1-5")
	Duration string `json:"duration"` // e.g. "8h", "30m"
	Timezone string `json:"timezone"` // e.g. "Asia/Seoul", "UTC"
}

type ScopeSpec struct {
	Namespaces    []string              `json:"namespaces,omitempty"`
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
}

type TargetLimitsSpec struct {
	MaxTargets    *int32 `json:"maxTargets,omitempty"`
	MaxPercentage *int32 `json:"maxPercentage,omitempty"`
}

type ProtectedResourcesSpec struct {
	Namespaces []string            `json:"namespaces,omitempty"`
	Labels     map[string]string   `json:"labels,omitempty"`
	Names      []ProtectedResource `json:"names,omitempty"`
}

type ProtectedResource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type ActionLimitsSpec struct {
	AllowedActions []string         `json:"allowedActions,omitempty"`
	MaxDuration    *metav1.Duration `json:"maxDuration,omitempty"`
}

// +kubebuilder:validation:Enum=Enforce;Audit
type EnforcementMode string

const (
	EnforcementEnforce EnforcementMode = "Enforce"
	EnforcementAudit   EnforcementMode = "Audit"
)

func init() {
	SchemeBuilder.Register(&BlastRadiusPolicy{}, &BlastRadiusPolicyList{})
}
