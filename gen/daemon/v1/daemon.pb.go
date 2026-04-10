package daemonv1

type NetworkChaosRequest struct {
	ExperimentId string            `json:"experiment_id,omitempty"`
	Action       string            `json:"action,omitempty"`
	TargetIface  string            `json:"target_iface,omitempty"`
	Parameters   map[string]string `json:"parameters,omitempty"`
}

func (x *NetworkChaosRequest) Reset()         {}
func (x *NetworkChaosRequest) String() string { return x.ExperimentId }
func (x *NetworkChaosRequest) ProtoMessage()  {}

func (x *NetworkChaosRequest) GetExperimentId() string          { return x.ExperimentId }
func (x *NetworkChaosRequest) GetAction() string                { return x.Action }
func (x *NetworkChaosRequest) GetTargetIface() string           { return x.TargetIface }
func (x *NetworkChaosRequest) GetParameters() map[string]string { return x.Parameters }

type NetworkChaosResponse struct {
	Success     bool   `json:"success,omitempty"`
	Message     string `json:"message,omitempty"`
	ExecutionId string `json:"execution_id,omitempty"`
}

func (x *NetworkChaosResponse) Reset()         {}
func (x *NetworkChaosResponse) String() string { return x.Message }
func (x *NetworkChaosResponse) ProtoMessage()  {}

func (x *NetworkChaosResponse) GetSuccess() bool       { return x.Success }
func (x *NetworkChaosResponse) GetMessage() string     { return x.Message }
func (x *NetworkChaosResponse) GetExecutionId() string { return x.ExecutionId }

type StressChaosRequest struct {
	ExperimentId string            `json:"experiment_id,omitempty"`
	StressorType string            `json:"stressor_type,omitempty"`
	Parameters   map[string]string `json:"parameters,omitempty"`
}

func (x *StressChaosRequest) Reset()         {}
func (x *StressChaosRequest) String() string { return x.ExperimentId }
func (x *StressChaosRequest) ProtoMessage()  {}

func (x *StressChaosRequest) GetExperimentId() string          { return x.ExperimentId }
func (x *StressChaosRequest) GetStressorType() string          { return x.StressorType }
func (x *StressChaosRequest) GetParameters() map[string]string { return x.Parameters }

type StressChaosResponse struct {
	Success     bool   `json:"success,omitempty"`
	Message     string `json:"message,omitempty"`
	ExecutionId string `json:"execution_id,omitempty"`
}

func (x *StressChaosResponse) Reset()         {}
func (x *StressChaosResponse) String() string { return x.Message }
func (x *StressChaosResponse) ProtoMessage()  {}

func (x *StressChaosResponse) GetSuccess() bool       { return x.Success }
func (x *StressChaosResponse) GetMessage() string     { return x.Message }
func (x *StressChaosResponse) GetExecutionId() string { return x.ExecutionId }

type DNSChaosRequest struct {
	ExperimentId string            `json:"experiment_id,omitempty"`
	Action       string            `json:"action,omitempty"`
	Parameters   map[string]string `json:"parameters,omitempty"`
}

func (x *DNSChaosRequest) Reset()         {}
func (x *DNSChaosRequest) String() string { return x.ExperimentId }
func (x *DNSChaosRequest) ProtoMessage()  {}

func (x *DNSChaosRequest) GetExperimentId() string          { return x.ExperimentId }
func (x *DNSChaosRequest) GetAction() string                { return x.Action }
func (x *DNSChaosRequest) GetParameters() map[string]string { return x.Parameters }

type DNSChaosResponse struct {
	Success     bool   `json:"success,omitempty"`
	Message     string `json:"message,omitempty"`
	ExecutionId string `json:"execution_id,omitempty"`
}

func (x *DNSChaosResponse) Reset()         {}
func (x *DNSChaosResponse) String() string { return x.Message }
func (x *DNSChaosResponse) ProtoMessage()  {}

func (x *DNSChaosResponse) GetSuccess() bool       { return x.Success }
func (x *DNSChaosResponse) GetMessage() string     { return x.Message }
func (x *DNSChaosResponse) GetExecutionId() string { return x.ExecutionId }

type HTTPChaosRequest struct {
	ExperimentId string            `json:"experiment_id,omitempty"`
	Action       string            `json:"action,omitempty"`
	Port         int32             `json:"port,omitempty"`
	Parameters   map[string]string `json:"parameters,omitempty"`
}

func (x *HTTPChaosRequest) Reset()         {}
func (x *HTTPChaosRequest) String() string { return x.ExperimentId }
func (x *HTTPChaosRequest) ProtoMessage()  {}

func (x *HTTPChaosRequest) GetExperimentId() string          { return x.ExperimentId }
func (x *HTTPChaosRequest) GetAction() string                { return x.Action }
func (x *HTTPChaosRequest) GetPort() int32                   { return x.Port }
func (x *HTTPChaosRequest) GetParameters() map[string]string { return x.Parameters }

type HTTPChaosResponse struct {
	Success     bool   `json:"success,omitempty"`
	Message     string `json:"message,omitempty"`
	ExecutionId string `json:"execution_id,omitempty"`
}

func (x *HTTPChaosResponse) Reset()         {}
func (x *HTTPChaosResponse) String() string { return x.Message }
func (x *HTTPChaosResponse) ProtoMessage()  {}

func (x *HTTPChaosResponse) GetSuccess() bool       { return x.Success }
func (x *HTTPChaosResponse) GetMessage() string     { return x.Message }
func (x *HTTPChaosResponse) GetExecutionId() string { return x.ExecutionId }

type NodeChaosRequest struct {
	ExperimentId string            `json:"experiment_id,omitempty"`
	Action       string            `json:"action,omitempty"`
	Parameters   map[string]string `json:"parameters,omitempty"`
}

func (x *NodeChaosRequest) Reset()         {}
func (x *NodeChaosRequest) String() string { return x.ExperimentId }
func (x *NodeChaosRequest) ProtoMessage()  {}

func (x *NodeChaosRequest) GetExperimentId() string          { return x.ExperimentId }
func (x *NodeChaosRequest) GetAction() string                { return x.Action }
func (x *NodeChaosRequest) GetParameters() map[string]string { return x.Parameters }

type NodeChaosResponse struct {
	Success     bool   `json:"success,omitempty"`
	Message     string `json:"message,omitempty"`
	ExecutionId string `json:"execution_id,omitempty"`
}

func (x *NodeChaosResponse) Reset()         {}
func (x *NodeChaosResponse) String() string { return x.Message }
func (x *NodeChaosResponse) ProtoMessage()  {}

func (x *NodeChaosResponse) GetSuccess() bool       { return x.Success }
func (x *NodeChaosResponse) GetMessage() string     { return x.Message }
func (x *NodeChaosResponse) GetExecutionId() string { return x.ExecutionId }

type CancelRequest struct {
	ExecutionId string `json:"execution_id,omitempty"`
}

func (x *CancelRequest) Reset()         {}
func (x *CancelRequest) String() string { return x.ExecutionId }
func (x *CancelRequest) ProtoMessage()  {}

func (x *CancelRequest) GetExecutionId() string { return x.ExecutionId }

type CancelResponse struct {
	Success bool   `json:"success,omitempty"`
	Message string `json:"message,omitempty"`
}

func (x *CancelResponse) Reset()         {}
func (x *CancelResponse) String() string { return x.Message }
func (x *CancelResponse) ProtoMessage()  {}

func (x *CancelResponse) GetSuccess() bool   { return x.Success }
func (x *CancelResponse) GetMessage() string { return x.Message }

type StatusRequest struct{}

func (x *StatusRequest) Reset()         {}
func (x *StatusRequest) String() string { return "StatusRequest" }
func (x *StatusRequest) ProtoMessage()  {}

type ExecutionStatus struct {
	ExecutionId  string `json:"execution_id,omitempty"`
	ExperimentId string `json:"experiment_id,omitempty"`
	Type         string `json:"type,omitempty"`
	Status       string `json:"status,omitempty"`
	StartTime    string `json:"start_time,omitempty"`
}

func (x *ExecutionStatus) Reset()         {}
func (x *ExecutionStatus) String() string { return x.ExecutionId }
func (x *ExecutionStatus) ProtoMessage()  {}

func (x *ExecutionStatus) GetExecutionId() string  { return x.ExecutionId }
func (x *ExecutionStatus) GetExperimentId() string { return x.ExperimentId }
func (x *ExecutionStatus) GetType() string         { return x.Type }
func (x *ExecutionStatus) GetStatus() string       { return x.Status }
func (x *ExecutionStatus) GetStartTime() string    { return x.StartTime }

type StatusResponse struct {
	Executions []*ExecutionStatus `json:"executions,omitempty"`
}

func (x *StatusResponse) Reset()         {}
func (x *StatusResponse) String() string { return "StatusResponse" }
func (x *StatusResponse) ProtoMessage()  {}

func (x *StatusResponse) GetExecutions() []*ExecutionStatus { return x.Executions }
