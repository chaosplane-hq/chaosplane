package daemon

import (
	"context"
	"fmt"
	"testing"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
)

type fakeRunner struct {
	runErr   error
	startErr error
}

func (f fakeRunner) Run(_ context.Context, _ string, _ ...string) (string, error) {
	return "", f.runErr
}

func (f fakeRunner) Start(_ string, _ ...string) error {
	return f.startErr
}

func TestExecChaosHonesty(t *testing.T) {
	cmdErr := fmt.Errorf("tc: command not found")

	tests := []struct {
		name        string
		runner      commandRunner
		call        func(*Server) (bool, string, string)
		wantSuccess bool
	}{
		{
			name:   "network non-ebpf failure reports false",
			runner: fakeRunner{runErr: cmdErr},
			call: func(s *Server) (bool, string, string) {
				resp, _ := s.ExecNetworkChaos(context.Background(), &daemonv1.NetworkChaosRequest{
					ExperimentId: "exp-1", Action: "delay", TargetIface: "eth0",
					Parameters: map[string]string{"latency": "100ms"},
				})
				return resp.GetSuccess(), resp.GetMessage(), resp.GetExecutionId()
			},
			wantSuccess: false,
		},
		{
			name:   "network non-ebpf success reports true",
			runner: fakeRunner{},
			call: func(s *Server) (bool, string, string) {
				resp, _ := s.ExecNetworkChaos(context.Background(), &daemonv1.NetworkChaosRequest{
					ExperimentId: "exp-1", Action: "delay", TargetIface: "eth0",
					Parameters: map[string]string{"latency": "100ms"},
				})
				return resp.GetSuccess(), resp.GetMessage(), resp.GetExecutionId()
			},
			wantSuccess: true,
		},
		{
			name:   "stress failure reports false",
			runner: fakeRunner{startErr: cmdErr},
			call: func(s *Server) (bool, string, string) {
				resp, _ := s.ExecStressChaos(context.Background(), &daemonv1.StressChaosRequest{
					ExperimentId: "exp-2", StressorType: "cpu",
					Parameters: map[string]string{"workers": "4"},
				})
				return resp.GetSuccess(), resp.GetMessage(), resp.GetExecutionId()
			},
			wantSuccess: false,
		},
		{
			name:   "stress success reports true",
			runner: fakeRunner{},
			call: func(s *Server) (bool, string, string) {
				resp, _ := s.ExecStressChaos(context.Background(), &daemonv1.StressChaosRequest{
					ExperimentId: "exp-2", StressorType: "cpu",
					Parameters: map[string]string{"workers": "4"},
				})
				return resp.GetSuccess(), resp.GetMessage(), resp.GetExecutionId()
			},
			wantSuccess: true,
		},
		{
			name:   "dns failure reports false",
			runner: fakeRunner{runErr: cmdErr},
			call: func(s *Server) (bool, string, string) {
				resp, _ := s.ExecDNSChaos(context.Background(), &daemonv1.DNSChaosRequest{
					ExperimentId: "exp-3", Action: "error",
					Parameters: map[string]string{"domains": "example.com"},
				})
				return resp.GetSuccess(), resp.GetMessage(), resp.GetExecutionId()
			},
			wantSuccess: false,
		},
		{
			name:   "dns success reports true",
			runner: fakeRunner{},
			call: func(s *Server) (bool, string, string) {
				resp, _ := s.ExecDNSChaos(context.Background(), &daemonv1.DNSChaosRequest{
					ExperimentId: "exp-3", Action: "error",
					Parameters: map[string]string{"domains": "example.com"},
				})
				return resp.GetSuccess(), resp.GetMessage(), resp.GetExecutionId()
			},
			wantSuccess: true,
		},
		{
			name:   "http abort failure reports false",
			runner: fakeRunner{runErr: cmdErr},
			call: func(s *Server) (bool, string, string) {
				resp, _ := s.ExecHTTPChaos(context.Background(), &daemonv1.HTTPChaosRequest{
					ExperimentId: "exp-4", Action: "abort", Port: 8080,
					Parameters: map[string]string{"code": "503"},
				})
				return resp.GetSuccess(), resp.GetMessage(), resp.GetExecutionId()
			},
			wantSuccess: false,
		},
		{
			name:   "http abort success reports true",
			runner: fakeRunner{},
			call: func(s *Server) (bool, string, string) {
				resp, _ := s.ExecHTTPChaos(context.Background(), &daemonv1.HTTPChaosRequest{
					ExperimentId: "exp-4", Action: "abort", Port: 8080,
					Parameters: map[string]string{"code": "503"},
				})
				return resp.GetSuccess(), resp.GetMessage(), resp.GetExecutionId()
			},
			wantSuccess: true,
		},
		{
			name:   "node cpu-stress failure reports false",
			runner: fakeRunner{startErr: cmdErr},
			call: func(s *Server) (bool, string, string) {
				resp, _ := s.ExecNodeChaos(context.Background(), &daemonv1.NodeChaosRequest{
					ExperimentId: "exp-5", Action: "cpu-stress",
					Parameters: map[string]string{"workers": "2"},
				})
				return resp.GetSuccess(), resp.GetMessage(), resp.GetExecutionId()
			},
			wantSuccess: false,
		},
		{
			name:   "node partition failure reports false",
			runner: fakeRunner{runErr: cmdErr},
			call: func(s *Server) (bool, string, string) {
				resp, _ := s.ExecNodeChaos(context.Background(), &daemonv1.NodeChaosRequest{
					ExperimentId: "exp-5", Action: "partition",
					Parameters: map[string]string{"iface": "eth0"},
				})
				return resp.GetSuccess(), resp.GetMessage(), resp.GetExecutionId()
			},
			wantSuccess: false,
		},
		{
			name:   "node cpu-stress success reports true",
			runner: fakeRunner{},
			call: func(s *Server) (bool, string, string) {
				resp, _ := s.ExecNodeChaos(context.Background(), &daemonv1.NodeChaosRequest{
					ExperimentId: "exp-5", Action: "cpu-stress",
					Parameters: map[string]string{"workers": "2"},
				})
				return resp.GetSuccess(), resp.GetMessage(), resp.GetExecutionId()
			},
			wantSuccess: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServerWithRunner(tc.runner)
			success, msg, execID := tc.call(srv)
			if success != tc.wantSuccess {
				t.Fatalf("Success = %v, want %v (message: %q)", success, tc.wantSuccess, msg)
			}
			if tc.wantSuccess {
				if execID == "" {
					t.Fatal("expected execution ID on success")
				}
			} else if msg == "" {
				t.Fatal("expected actionable message on failure")
			}
		})
	}
}
