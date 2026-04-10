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
	ExecDNSChaos(ctx context.Context, in *DNSChaosRequest, opts ...grpc.CallOption) (*DNSChaosResponse, error)
	ExecHTTPChaos(ctx context.Context, in *HTTPChaosRequest, opts ...grpc.CallOption) (*HTTPChaosResponse, error)
	ExecNodeChaos(ctx context.Context, in *NodeChaosRequest, opts ...grpc.CallOption) (*NodeChaosResponse, error)
	CancelChaos(ctx context.Context, in *CancelRequest, opts ...grpc.CallOption) (*CancelResponse, error)
	GetChaosStatus(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error)
}

type ChaosDaemonServer interface {
	ExecNetworkChaos(context.Context, *NetworkChaosRequest) (*NetworkChaosResponse, error)
	ExecStressChaos(context.Context, *StressChaosRequest) (*StressChaosResponse, error)
	ExecDNSChaos(context.Context, *DNSChaosRequest) (*DNSChaosResponse, error)
	ExecHTTPChaos(context.Context, *HTTPChaosRequest) (*HTTPChaosResponse, error)
	ExecNodeChaos(context.Context, *NodeChaosRequest) (*NodeChaosResponse, error)
	CancelChaos(context.Context, *CancelRequest) (*CancelResponse, error)
	GetChaosStatus(context.Context, *StatusRequest) (*StatusResponse, error)
	mustEmbedUnimplementedChaosDaemonServer()
}

type UnimplementedChaosDaemonServer struct{}

func (UnimplementedChaosDaemonServer) ExecNetworkChaos(context.Context, *NetworkChaosRequest) (*NetworkChaosResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ExecNetworkChaos not implemented")
}

func (UnimplementedChaosDaemonServer) ExecStressChaos(context.Context, *StressChaosRequest) (*StressChaosResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ExecStressChaos not implemented")
}

func (UnimplementedChaosDaemonServer) ExecDNSChaos(context.Context, *DNSChaosRequest) (*DNSChaosResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ExecDNSChaos not implemented")
}

func (UnimplementedChaosDaemonServer) ExecHTTPChaos(context.Context, *HTTPChaosRequest) (*HTTPChaosResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ExecHTTPChaos not implemented")
}

func (UnimplementedChaosDaemonServer) ExecNodeChaos(context.Context, *NodeChaosRequest) (*NodeChaosResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ExecNodeChaos not implemented")
}

func (UnimplementedChaosDaemonServer) CancelChaos(context.Context, *CancelRequest) (*CancelResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CancelChaos not implemented")
}

func (UnimplementedChaosDaemonServer) GetChaosStatus(context.Context, *StatusRequest) (*StatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetChaosStatus not implemented")
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
			MethodName: "ExecDNSChaos",
			Handler:    _ChaosDaemon_ExecDNSChaos_Handler,
		},
		{
			MethodName: "ExecHTTPChaos",
			Handler:    _ChaosDaemon_ExecHTTPChaos_Handler,
		},
		{
			MethodName: "ExecNodeChaos",
			Handler:    _ChaosDaemon_ExecNodeChaos_Handler,
		},
		{
			MethodName: "CancelChaos",
			Handler:    _ChaosDaemon_CancelChaos_Handler,
		},
		{
			MethodName: "GetChaosStatus",
			Handler:    _ChaosDaemon_GetChaosStatus_Handler,
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

func _ChaosDaemon_ExecDNSChaos_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DNSChaosRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChaosDaemonServer).ExecDNSChaos(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + ChaosDaemon_ServiceName + "/ExecDNSChaos",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChaosDaemonServer).ExecDNSChaos(ctx, req.(*DNSChaosRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ChaosDaemon_ExecHTTPChaos_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HTTPChaosRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChaosDaemonServer).ExecHTTPChaos(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + ChaosDaemon_ServiceName + "/ExecHTTPChaos",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChaosDaemonServer).ExecHTTPChaos(ctx, req.(*HTTPChaosRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ChaosDaemon_ExecNodeChaos_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(NodeChaosRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChaosDaemonServer).ExecNodeChaos(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + ChaosDaemon_ServiceName + "/ExecNodeChaos",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChaosDaemonServer).ExecNodeChaos(ctx, req.(*NodeChaosRequest))
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

func _ChaosDaemon_GetChaosStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChaosDaemonServer).GetChaosStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + ChaosDaemon_ServiceName + "/GetChaosStatus",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChaosDaemonServer).GetChaosStatus(ctx, req.(*StatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}
