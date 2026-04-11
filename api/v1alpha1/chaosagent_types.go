package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.connectionStatus`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.agentVersion`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environmentRef.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ChaosAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChaosAgentSpec   `json:"spec,omitempty"`
	Status ChaosAgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ChaosAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChaosAgent `json:"items"`
}

type ChaosAgentSpec struct {
	EnvironmentRef    AgentEnvironmentRef `json:"environmentRef"`
	PlatformURL       string              `json:"platformUrl"`
	TokenSecretRef    SecretKeyRef        `json:"tokenSecretRef"`
	HeartbeatInterval metav1.Duration     `json:"heartbeatInterval,omitempty"`
	TLS               *AgentTLSSpec       `json:"tls,omitempty"`
	TargetType        AgentTargetType     `json:"targetType,omitempty"`
	VMTargets         []VMTarget          `json:"vmTargets,omitempty"`
}

// +kubebuilder:validation:Enum=kubernetes;vm;hybrid
type AgentTargetType string

const (
	AgentTargetKubernetes AgentTargetType = "kubernetes"
	AgentTargetVM         AgentTargetType = "vm"
	AgentTargetHybrid     AgentTargetType = "hybrid"
)

type VMTarget struct {
	Host      string            `json:"host"`
	Port      int               `json:"port,omitempty"`
	User      string            `json:"user,omitempty"`
	KeySecret string            `json:"keySecret,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type AgentEnvironmentRef struct {
	Name string `json:"name"`
	ID   string `json:"id,omitempty"`
}

type SecretKeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type AgentTLSSpec struct {
	Enabled    bool   `json:"enabled,omitempty"`
	SecretName string `json:"secretName,omitempty"`
}

// +kubebuilder:validation:Enum=connected;disconnected;degraded;registering;error
type AgentConnectionStatus string

const (
	AgentStatusConnected    AgentConnectionStatus = "connected"
	AgentStatusDisconnected AgentConnectionStatus = "disconnected"
	AgentStatusDegraded     AgentConnectionStatus = "degraded"
	AgentStatusRegistering  AgentConnectionStatus = "registering"
	AgentStatusError        AgentConnectionStatus = "error"
)

type ChaosAgentStatus struct {
	ConnectionStatus   AgentConnectionStatus `json:"connectionStatus,omitempty"`
	AgentVersion       string                `json:"agentVersion,omitempty"`
	LastHeartbeat      *metav1.Time          `json:"lastHeartbeat,omitempty"`
	RegisteredAt       *metav1.Time          `json:"registeredAt,omitempty"`
	ClusterInfo        *ClusterInfo          `json:"clusterInfo,omitempty"`
	Conditions         []metav1.Condition    `json:"conditions,omitempty"`
	ObservedGeneration int64                 `json:"observedGeneration,omitempty"`
}

type ClusterInfo struct {
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`
	Platform          string `json:"platform,omitempty"`
	NodeCount         int    `json:"nodeCount,omitempty"`
	Region            string `json:"region,omitempty"`
}

func init() {
	SchemeBuilder.Register(&ChaosAgent{}, &ChaosAgentList{})
}
