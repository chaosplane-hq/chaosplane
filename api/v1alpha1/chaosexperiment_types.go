package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target.kind`
// +kubebuilder:printcolumn:name="Action",type=string,JSONPath=`.spec.action.type`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ChaosExperiment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChaosExperimentSpec   `json:"spec,omitempty"`
	Status ChaosExperimentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ChaosExperimentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChaosExperiment `json:"items"`
}

type ChaosExperimentSpec struct {
	Target      TargetSpec       `json:"target"`
	Action      ActionSpec       `json:"action"`
	Duration    metav1.Duration  `json:"duration"`
	Rollback    *RollbackSpec    `json:"rollback,omitempty"`
	Execution   ExecutionSpec    `json:"execution,omitempty"`
	SteadyState *SteadyStateSpec `json:"steadyState,omitempty"`
}

type SteadyStateSpec struct {
	Before          []ProbeSpec     `json:"before,omitempty"`
	After           []ProbeSpec     `json:"after,omitempty"`
	RecoveryTimeout metav1.Duration `json:"recoveryTimeout,omitempty"`
}

// +kubebuilder:validation:Enum=prometheus;http;k8s
type ProbeType string

const (
	ProbeTypePrometheus ProbeType = "prometheus"
	ProbeTypeHTTP       ProbeType = "http"
	ProbeTypeK8s        ProbeType = "k8s"
)

type ProbeSpec struct {
	Name       string           `json:"name"`
	Type       ProbeType        `json:"type"`
	Prometheus *PrometheusProbe `json:"prometheus,omitempty"`
	HTTP       *HTTPProbe       `json:"http,omitempty"`
	K8s        *K8sProbe        `json:"k8s,omitempty"`
}

type PrometheusProbe struct {
	URL       string         `json:"url"`
	Query     string         `json:"query"`
	Condition ProbeCondition `json:"condition"`
}

type ProbeCondition struct {
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
}

type HTTPProbe struct {
	URL            string `json:"url"`
	Method         string `json:"method,omitempty"`
	ExpectedStatus int    `json:"expectedStatus,omitempty"`
	ExpectedBody   string `json:"expectedBody,omitempty"`
}

type K8sProbe struct {
	Resource      string            `json:"resource"`
	Namespace     string            `json:"namespace,omitempty"`
	LabelSelector string            `json:"labelSelector,omitempty"`
	FieldSelector string            `json:"fieldSelector,omitempty"`
	Condition     K8sProbeCondition `json:"condition"`
}

type K8sProbeCondition struct {
	MinReady int `json:"minReady"`
}

type TargetSpec struct {
	Kind          string                `json:"kind"`
	Namespace     string                `json:"namespace,omitempty"`
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
	Names         []string              `json:"names,omitempty"`
}

type ActionSpec struct {
	Type       string               `json:"type"`
	Parameters runtime.RawExtension `json:"parameters,omitempty"`
	Duration   *metav1.Duration     `json:"duration,omitempty"`
}

type RollbackSpec struct {
	Enabled bool             `json:"enabled"`
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

type ExecutionSpec struct {
	Parallelism *int32 `json:"parallelism,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;SteadyStateChecking;Running;Completing;Recovering;Completed;Failed;Aborted
type ExperimentPhase string

const (
	PhasePending             ExperimentPhase = "Pending"
	PhaseSteadyStateChecking ExperimentPhase = "SteadyStateChecking"
	PhaseRunning             ExperimentPhase = "Running"
	PhaseCompleting          ExperimentPhase = "Completing"
	PhaseRecovering          ExperimentPhase = "Recovering"
	PhaseCompleted           ExperimentPhase = "Completed"
	PhaseFailed              ExperimentPhase = "Failed"
	PhaseAborted             ExperimentPhase = "Aborted"
)

type ChaosExperimentStatus struct {
	Phase              ExperimentPhase    `json:"phase,omitempty"`
	StartTime          *metav1.Time       `json:"startTime,omitempty"`
	EndTime            *metav1.Time       `json:"endTime,omitempty"`
	RecoveryStartTime  *metav1.Time       `json:"recoveryStartTime,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	AffectedResources  []string           `json:"affectedResources,omitempty"`
	Message            string             `json:"message,omitempty"`
}

func init() {
	SchemeBuilder.Register(&ChaosExperiment{}, &ChaosExperimentList{})
}
