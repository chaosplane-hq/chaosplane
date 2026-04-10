package daemonv1

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const ChaosDaemon_ServiceName = "daemon.v1.ChaosDaemon"

type ChaosDaemonClient interface {
	ExecNetworkChaos(ctx context.Context, in *NetworkChaosRequest, opts ...grpc.CallOption) (*NetworkChaosResponse, error)
	ExecStressChaos(ctx context.Context, in *StressChaosRequest, opts ...grpc.CallOption) (*StressChaosResponse, error)
	CancelChaos(ctx context.Context, in *CancelRequest, opts ...grpc.CallOption) (*CancelResponse, error)
}

type ChaosDaemonServer interface {
	ExecNetworkChaos(context.Context, *NetworkChaosRequest) (*NetworkChaosResponse, error)
	ExecStressChaos(context.Context, *StressChaosRequest) (*StressChaosResponse, error)
	CancelChaos(context.Context, *CancelRequest) (*CancelResponse, error)
	mustEmbedUnimplementedChaosDaemonServer()
}

type UnimplementedChaosDaemonServer struct{}

func (UnimplementedChaosDaemonServer) ExecNetworkChaos(context.Context, *NetworkChaosRequest) (*NetworkChaosResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ExecNetworkChaos not implemented")
}

func (UnimplementedChaosDaemonServer) ExecStressChaos(context.Context, *StressChaosRequest) (*StressChaosResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ExecStressChaos not implemented")
}

func (UnimplementedChaosDaemonServer) CancelChaos(context.Context, *CancelRequest) (*CancelResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CancelChaos not implemented")
}

func (UnimplementedChaosDaemonServer) mustEmbedUnimplementedChaosDaemonServer() {}

type UnsafeChaosDaemonServer interface {
	mustEmbedUnimplementedChaosDaemonServer()
}

var ChaosDaemon_ServiceDesc = grpc.ServiceDesc{
	ServiceName: ChaosDaemon_ServiceName,
	HandlerType: (*ChaosDaemonServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ExecNetworkChaos",
			Handler:    _ChaosDaemon_ExecNetworkChaos_Handler,
		},
		{
			MethodName: "ExecStressChaos",
			Handler:    _ChaosDaemon_ExecStressChaos_Handler,
		},
		{
			MethodName: "CancelChaos",
			Handler:    _ChaosDaemon_CancelChaos_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "daemon/v1/daemon.proto",
}

func RegisterChaosDaemonServer(s grpc.ServiceRegistrar, srv ChaosDaemonServer) {
	s.RegisterService(&ChaosDaemon_ServiceDesc, srv)
}

func _ChaosDaemon_ExecNetworkChaos_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(NetworkChaosRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChaosDaemonServer).ExecNetworkChaos(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + ChaosDaemon_ServiceName + "/ExecNetworkChaos",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChaosDaemonServer).ExecNetworkChaos(ctx, req.(*NetworkChaosRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ChaosDaemon_ExecStressChaos_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StressChaosRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChaosDaemonServer).ExecStressChaos(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + ChaosDaemon_ServiceName + "/ExecStressChaos",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChaosDaemonServer).ExecStressChaos(ctx, req.(*StressChaosRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ChaosDaemon_CancelChaos_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CancelRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChaosDaemonServer).CancelChaos(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + ChaosDaemon_ServiceName + "/CancelChaos",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChaosDaemonServer).CancelChaos(ctx, req.(*CancelRequest))
	}
	return interceptor(ctx, in, info, handler)
}
