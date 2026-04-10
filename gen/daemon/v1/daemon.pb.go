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
	Success bool   `json:"success,omitempty"`
	Message string `json:"message,omitempty"`
}

func (x *NetworkChaosResponse) Reset()         {}
func (x *NetworkChaosResponse) String() string { return x.Message }
func (x *NetworkChaosResponse) ProtoMessage()  {}

func (x *NetworkChaosResponse) GetSuccess() bool   { return x.Success }
func (x *NetworkChaosResponse) GetMessage() string { return x.Message }

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
	Success bool   `json:"success,omitempty"`
	Message string `json:"message,omitempty"`
}

func (x *StressChaosResponse) Reset()         {}
func (x *StressChaosResponse) String() string { return x.Message }
func (x *StressChaosResponse) ProtoMessage()  {}

func (x *StressChaosResponse) GetSuccess() bool   { return x.Success }
func (x *StressChaosResponse) GetMessage() string { return x.Message }

type CancelRequest struct {
	ExperimentId string `json:"experiment_id,omitempty"`
}

func (x *CancelRequest) Reset()         {}
func (x *CancelRequest) String() string { return x.ExperimentId }
func (x *CancelRequest) ProtoMessage()  {}

func (x *CancelRequest) GetExperimentId() string { return x.ExperimentId }

type CancelResponse struct {
	Success bool   `json:"success,omitempty"`
	Message string `json:"message,omitempty"`
}

func (x *CancelResponse) Reset()         {}
func (x *CancelResponse) String() string { return x.Message }
func (x *CancelResponse) ProtoMessage()  {}

func (x *CancelResponse) GetSuccess() bool   { return x.Success }
func (x *CancelResponse) GetMessage() string { return x.Message }
