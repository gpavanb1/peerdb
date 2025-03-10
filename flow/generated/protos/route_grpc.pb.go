// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v4.23.2
// source: route.proto

package protos

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

const (
	FlowService_CreatePeerFlow_FullMethodName = "/peerdb_route.FlowService/CreatePeerFlow"
	FlowService_CreateQRepFlow_FullMethodName = "/peerdb_route.FlowService/CreateQRepFlow"
	FlowService_HealthCheck_FullMethodName    = "/peerdb_route.FlowService/HealthCheck"
	FlowService_ShutdownFlow_FullMethodName   = "/peerdb_route.FlowService/ShutdownFlow"
)

// FlowServiceClient is the client API for FlowService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type FlowServiceClient interface {
	CreatePeerFlow(ctx context.Context, in *CreatePeerFlowRequest, opts ...grpc.CallOption) (*CreatePeerFlowResponse, error)
	CreateQRepFlow(ctx context.Context, in *CreateQRepFlowRequest, opts ...grpc.CallOption) (*CreateQRepFlowResponse, error)
	HealthCheck(ctx context.Context, in *HealthCheckRequest, opts ...grpc.CallOption) (*HealthCheckResponse, error)
	ShutdownFlow(ctx context.Context, in *ShutdownRequest, opts ...grpc.CallOption) (*ShutdownResponse, error)
}

type flowServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewFlowServiceClient(cc grpc.ClientConnInterface) FlowServiceClient {
	return &flowServiceClient{cc}
}

func (c *flowServiceClient) CreatePeerFlow(ctx context.Context, in *CreatePeerFlowRequest, opts ...grpc.CallOption) (*CreatePeerFlowResponse, error) {
	out := new(CreatePeerFlowResponse)
	err := c.cc.Invoke(ctx, FlowService_CreatePeerFlow_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *flowServiceClient) CreateQRepFlow(ctx context.Context, in *CreateQRepFlowRequest, opts ...grpc.CallOption) (*CreateQRepFlowResponse, error) {
	out := new(CreateQRepFlowResponse)
	err := c.cc.Invoke(ctx, FlowService_CreateQRepFlow_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *flowServiceClient) HealthCheck(ctx context.Context, in *HealthCheckRequest, opts ...grpc.CallOption) (*HealthCheckResponse, error) {
	out := new(HealthCheckResponse)
	err := c.cc.Invoke(ctx, FlowService_HealthCheck_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *flowServiceClient) ShutdownFlow(ctx context.Context, in *ShutdownRequest, opts ...grpc.CallOption) (*ShutdownResponse, error) {
	out := new(ShutdownResponse)
	err := c.cc.Invoke(ctx, FlowService_ShutdownFlow_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FlowServiceServer is the server API for FlowService service.
// All implementations must embed UnimplementedFlowServiceServer
// for forward compatibility
type FlowServiceServer interface {
	CreatePeerFlow(context.Context, *CreatePeerFlowRequest) (*CreatePeerFlowResponse, error)
	CreateQRepFlow(context.Context, *CreateQRepFlowRequest) (*CreateQRepFlowResponse, error)
	HealthCheck(context.Context, *HealthCheckRequest) (*HealthCheckResponse, error)
	ShutdownFlow(context.Context, *ShutdownRequest) (*ShutdownResponse, error)
	mustEmbedUnimplementedFlowServiceServer()
}

// UnimplementedFlowServiceServer must be embedded to have forward compatible implementations.
type UnimplementedFlowServiceServer struct {
}

func (UnimplementedFlowServiceServer) CreatePeerFlow(context.Context, *CreatePeerFlowRequest) (*CreatePeerFlowResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreatePeerFlow not implemented")
}
func (UnimplementedFlowServiceServer) CreateQRepFlow(context.Context, *CreateQRepFlowRequest) (*CreateQRepFlowResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateQRepFlow not implemented")
}
func (UnimplementedFlowServiceServer) HealthCheck(context.Context, *HealthCheckRequest) (*HealthCheckResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method HealthCheck not implemented")
}
func (UnimplementedFlowServiceServer) ShutdownFlow(context.Context, *ShutdownRequest) (*ShutdownResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ShutdownFlow not implemented")
}
func (UnimplementedFlowServiceServer) mustEmbedUnimplementedFlowServiceServer() {}

// UnsafeFlowServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to FlowServiceServer will
// result in compilation errors.
type UnsafeFlowServiceServer interface {
	mustEmbedUnimplementedFlowServiceServer()
}

func RegisterFlowServiceServer(s grpc.ServiceRegistrar, srv FlowServiceServer) {
	s.RegisterService(&FlowService_ServiceDesc, srv)
}

func _FlowService_CreatePeerFlow_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreatePeerFlowRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FlowServiceServer).CreatePeerFlow(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FlowService_CreatePeerFlow_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FlowServiceServer).CreatePeerFlow(ctx, req.(*CreatePeerFlowRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FlowService_CreateQRepFlow_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateQRepFlowRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FlowServiceServer).CreateQRepFlow(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FlowService_CreateQRepFlow_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FlowServiceServer).CreateQRepFlow(ctx, req.(*CreateQRepFlowRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FlowService_HealthCheck_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HealthCheckRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FlowServiceServer).HealthCheck(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FlowService_HealthCheck_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FlowServiceServer).HealthCheck(ctx, req.(*HealthCheckRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FlowService_ShutdownFlow_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ShutdownRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FlowServiceServer).ShutdownFlow(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FlowService_ShutdownFlow_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FlowServiceServer).ShutdownFlow(ctx, req.(*ShutdownRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// FlowService_ServiceDesc is the grpc.ServiceDesc for FlowService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var FlowService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "peerdb_route.FlowService",
	HandlerType: (*FlowServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CreatePeerFlow",
			Handler:    _FlowService_CreatePeerFlow_Handler,
		},
		{
			MethodName: "CreateQRepFlow",
			Handler:    _FlowService_CreateQRepFlow_Handler,
		},
		{
			MethodName: "HealthCheck",
			Handler:    _FlowService_HealthCheck_Handler,
		},
		{
			MethodName: "ShutdownFlow",
			Handler:    _FlowService_ShutdownFlow_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "route.proto",
}
