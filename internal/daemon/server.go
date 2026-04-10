package daemon

import (
	"context"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	daemonv1.UnimplementedChaosDaemonServer
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) ExecNetworkChaos(_ context.Context, _ *daemonv1.NetworkChaosRequest) (*daemonv1.NetworkChaosResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ExecNetworkChaos not implemented")
}

func (s *Server) ExecStressChaos(_ context.Context, _ *daemonv1.StressChaosRequest) (*daemonv1.StressChaosResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ExecStressChaos not implemented")
}

func (s *Server) CancelChaos(_ context.Context, _ *daemonv1.CancelRequest) (*daemonv1.CancelResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "CancelChaos not implemented")
}
