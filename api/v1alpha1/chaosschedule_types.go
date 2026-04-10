package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspended`
// +kubebuilder:printcolumn:name="Last Run",type=date,JSONPath=`.status.lastScheduleTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ChaosSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChaosScheduleSpec   `json:"spec,omitempty"`
	Status ChaosScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ChaosScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChaosSchedule `json:"items"`
}

type ChaosScheduleSpec struct {
	Schedule                string               `json:"schedule"`
	Timezone                string               `json:"timezone,omitempty"`
	Suspended               bool                 `json:"suspended,omitempty"`
	ConcurrencyPolicy       ConcurrencyPolicy    `json:"concurrencyPolicy,omitempty"`
	StartingDeadlineSeconds *int64               `json:"startingDeadlineSeconds,omitempty"`
	SuccessfulRunsLimit     *int32               `json:"successfulRunsLimit,omitempty"`
	FailedRunsLimit         *int32               `json:"failedRunsLimit,omitempty"`
	ExperimentTemplate      *ChaosExperimentSpec `json:"experimentTemplate,omitempty"`
	WorkflowTemplate        *ChaosWorkflowSpec   `json:"workflowTemplate,omitempty"`
}

// +kubebuilder:validation:Enum=Allow;Forbid;Replace
type ConcurrencyPolicy string

const (
	ConcurrencyAllow   ConcurrencyPolicy = "Allow"
	ConcurrencyForbid  ConcurrencyPolicy = "Forbid"
	ConcurrencyReplace ConcurrencyPolicy = "Replace"
)

type ChaosScheduleStatus struct {
	LastScheduleTime   *metav1.Time       `json:"lastScheduleTime,omitempty"`
	NextScheduleTime   *metav1.Time       `json:"nextScheduleTime,omitempty"`
	ActiveRuns         []ScheduleRunRef   `json:"activeRuns,omitempty"`
	SuccessfulRuns     int32              `json:"successfulRuns,omitempty"`
	FailedRuns         int32              `json:"failedRuns,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

type ScheduleRunRef struct {
	Name      string       `json:"name"`
	Namespace string       `json:"namespace,omitempty"`
	Kind      string       `json:"kind"`
	StartTime *metav1.Time `json:"startTime,omitempty"`
}

func init() {
	SchemeBuilder.Register(&ChaosSchedule{}, &ChaosScheduleList{})
}
