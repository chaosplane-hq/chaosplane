package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Progress",type=string,JSONPath=`.status.progress`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ChaosWorkflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChaosWorkflowSpec   `json:"spec,omitempty"`
	Status ChaosWorkflowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ChaosWorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChaosWorkflow `json:"items"`
}

type ChaosWorkflowSpec struct {
	Templates     []WorkflowTemplate    `json:"templates"`
	Parameters    []WorkflowParameter   `json:"parameters,omitempty"`
	ErrorHandling ErrorHandlingSpec     `json:"errorHandling,omitempty"`
	Execution     WorkflowExecutionSpec `json:"execution,omitempty"`
}

// +kubebuilder:validation:Enum=experiment;delay;condition;parallel;suspend
type TemplateType string

const (
	TemplateTypeExperiment TemplateType = "experiment"
	TemplateTypeDelay      TemplateType = "delay"
	TemplateTypeCondition  TemplateType = "condition"
	TemplateTypeParallel   TemplateType = "parallel"
	TemplateTypeSuspend    TemplateType = "suspend"
)

type WorkflowTemplate struct {
	Name          string         `json:"name"`
	Type          TemplateType   `json:"type"`
	ExperimentRef *ExperimentRef `json:"experimentRef,omitempty"`
	Delay         *DelaySpec     `json:"delay,omitempty"`
	Condition     *ConditionSpec `json:"condition,omitempty"`
	Parallel      *ParallelSpec  `json:"parallel,omitempty"`
	Suspend       *SuspendSpec   `json:"suspend,omitempty"`
	Dependencies  []string       `json:"dependencies,omitempty"`
}

type ExperimentRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type DelaySpec struct {
	Duration metav1.Duration `json:"duration"`
}

type ConditionSpec struct {
	Expression string `json:"expression"`
	IfTrue     string `json:"ifTrue,omitempty"`
	IfFalse    string `json:"ifFalse,omitempty"`
}

type ParallelSpec struct {
	Templates []string `json:"templates"`
}

type SuspendSpec struct {
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

type WorkflowParameter struct {
	Name    string `json:"name"`
	Default string `json:"default,omitempty"`
	Value   string `json:"value,omitempty"`
}

// +kubebuilder:validation:Enum=abort;continue;retry;rollback
type ErrorHandlingStrategy string

const (
	ErrorStrategyAbort    ErrorHandlingStrategy = "abort"
	ErrorStrategyContinue ErrorHandlingStrategy = "continue"
	ErrorStrategyRetry    ErrorHandlingStrategy = "retry"
	ErrorStrategyRollback ErrorHandlingStrategy = "rollback"
)

type ErrorHandlingSpec struct {
	Strategy ErrorHandlingStrategy `json:"strategy,omitempty"`
}

type WorkflowExecutionSpec struct {
	MaxParallelism *int32 `json:"maxParallelism,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;Running;Paused;Completed;Failed;Aborted
type WorkflowPhase string

const (
	WorkflowPhasePending   WorkflowPhase = "Pending"
	WorkflowPhaseRunning   WorkflowPhase = "Running"
	WorkflowPhasePaused    WorkflowPhase = "Paused"
	WorkflowPhaseCompleted WorkflowPhase = "Completed"
	WorkflowPhaseFailed    WorkflowPhase = "Failed"
	WorkflowPhaseAborted   WorkflowPhase = "Aborted"
)

type ChaosWorkflowStatus struct {
	Phase              WorkflowPhase      `json:"phase,omitempty"`
	StartTime          *metav1.Time       `json:"startTime,omitempty"`
	EndTime            *metav1.Time       `json:"endTime,omitempty"`
	TemplateStatuses   []TemplateStatus   `json:"templateStatuses,omitempty"`
	Progress           string             `json:"progress,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

type TemplateStatus struct {
	Name      string        `json:"name"`
	Phase     WorkflowPhase `json:"phase,omitempty"`
	StartTime *metav1.Time  `json:"startTime,omitempty"`
	EndTime   *metav1.Time  `json:"endTime,omitempty"`
	Message   string        `json:"message,omitempty"`
}

func init() {
	SchemeBuilder.Register(&ChaosWorkflow{}, &ChaosWorkflowList{})
}
