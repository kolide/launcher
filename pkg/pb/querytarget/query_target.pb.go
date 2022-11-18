// Code generated by protoc-gen-go. DO NOT EDIT.
// source: query_target.proto

package querytarget

import (
	fmt "fmt"
	math "math"

	proto "github.com/golang/protobuf/proto"

	context "golang.org/x/net/context"

	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type GetTargetsRequest struct {
	NodeKey              string   `protobuf:"bytes,1,opt,name=node_key,json=nodeKey,proto3" json:"node_key,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetTargetsRequest) Reset()         { *m = GetTargetsRequest{} }
func (m *GetTargetsRequest) String() string { return proto.CompactTextString(m) }
func (*GetTargetsRequest) ProtoMessage()    {}
func (*GetTargetsRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_1d18d31898835767, []int{0}
}

func (m *GetTargetsRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetTargetsRequest.Unmarshal(m, b)
}
func (m *GetTargetsRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetTargetsRequest.Marshal(b, m, deterministic)
}
func (m *GetTargetsRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetTargetsRequest.Merge(m, src)
}
func (m *GetTargetsRequest) XXX_Size() int {
	return xxx_messageInfo_GetTargetsRequest.Size(m)
}
func (m *GetTargetsRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetTargetsRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetTargetsRequest proto.InternalMessageInfo

func (m *GetTargetsRequest) GetNodeKey() string {
	if m != nil {
		return m.NodeKey
	}
	return ""
}

type GetTargetsResponse struct {
	Targets              []*Target `protobuf:"bytes,1,rep,name=targets,proto3" json:"targets,omitempty"`
	XXX_NoUnkeyedLiteral struct{}  `json:"-"`
	XXX_unrecognized     []byte    `json:"-"`
	XXX_sizecache        int32     `json:"-"`
}

func (m *GetTargetsResponse) Reset()         { *m = GetTargetsResponse{} }
func (m *GetTargetsResponse) String() string { return proto.CompactTextString(m) }
func (*GetTargetsResponse) ProtoMessage()    {}
func (*GetTargetsResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_1d18d31898835767, []int{1}
}

func (m *GetTargetsResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetTargetsResponse.Unmarshal(m, b)
}
func (m *GetTargetsResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetTargetsResponse.Marshal(b, m, deterministic)
}
func (m *GetTargetsResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetTargetsResponse.Merge(m, src)
}
func (m *GetTargetsResponse) XXX_Size() int {
	return xxx_messageInfo_GetTargetsResponse.Size(m)
}
func (m *GetTargetsResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_GetTargetsResponse.DiscardUnknown(m)
}

var xxx_messageInfo_GetTargetsResponse proto.InternalMessageInfo

func (m *GetTargetsResponse) GetTargets() []*Target {
	if m != nil {
		return m.Targets
	}
	return nil
}

type Target struct {
	Id                   string   `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Target) Reset()         { *m = Target{} }
func (m *Target) String() string { return proto.CompactTextString(m) }
func (*Target) ProtoMessage()    {}
func (*Target) Descriptor() ([]byte, []int) {
	return fileDescriptor_1d18d31898835767, []int{2}
}

func (m *Target) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Target.Unmarshal(m, b)
}
func (m *Target) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Target.Marshal(b, m, deterministic)
}
func (m *Target) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Target.Merge(m, src)
}
func (m *Target) XXX_Size() int {
	return xxx_messageInfo_Target.Size(m)
}
func (m *Target) XXX_DiscardUnknown() {
	xxx_messageInfo_Target.DiscardUnknown(m)
}

var xxx_messageInfo_Target proto.InternalMessageInfo

func (m *Target) GetId() string {
	if m != nil {
		return m.Id
	}
	return ""
}

func init() {
	proto.RegisterType((*GetTargetsRequest)(nil), "kolide.launcher.GetTargetsRequest")
	proto.RegisterType((*GetTargetsResponse)(nil), "kolide.launcher.GetTargetsResponse")
	proto.RegisterType((*Target)(nil), "kolide.launcher.Target")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// QueryTargetClient is the client API for QueryTarget service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type QueryTargetClient interface {
	GetTargets(ctx context.Context, in *GetTargetsRequest, opts ...grpc.CallOption) (*GetTargetsResponse, error)
}

type queryTargetClient struct {
	cc *grpc.ClientConn
}

func NewQueryTargetClient(cc *grpc.ClientConn) QueryTargetClient {
	return &queryTargetClient{cc}
}

func (c *queryTargetClient) GetTargets(ctx context.Context, in *GetTargetsRequest, opts ...grpc.CallOption) (*GetTargetsResponse, error) {
	out := new(GetTargetsResponse)
	err := c.cc.Invoke(ctx, "/kolide.launcher.QueryTarget/GetTargets", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// QueryTargetServer is the server API for QueryTarget service.
type QueryTargetServer interface {
	GetTargets(context.Context, *GetTargetsRequest) (*GetTargetsResponse, error)
}

func RegisterQueryTargetServer(s *grpc.Server, srv QueryTargetServer) {
	s.RegisterService(&_QueryTarget_serviceDesc, srv)
}

func _QueryTarget_GetTargets_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetTargetsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(QueryTargetServer).GetTargets(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/kolide.launcher.QueryTarget/GetTargets",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(QueryTargetServer).GetTargets(ctx, req.(*GetTargetsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _QueryTarget_serviceDesc = grpc.ServiceDesc{
	ServiceName: "kolide.launcher.QueryTarget",
	HandlerType: (*QueryTargetServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetTargets",
			Handler:    _QueryTarget_GetTargets_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "query_target.proto",
}

func init() { proto.RegisterFile("query_target.proto", fileDescriptor_1d18d31898835767) }

var fileDescriptor_1d18d31898835767 = []byte{
	// 201 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x12, 0x2a, 0x2c, 0x4d, 0x2d,
	0xaa, 0x8c, 0x2f, 0x49, 0x2c, 0x4a, 0x4f, 0x2d, 0xd1, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0xe2,
	0xcf, 0xce, 0xcf, 0xc9, 0x4c, 0x49, 0xd5, 0xcb, 0x49, 0x2c, 0xcd, 0x4b, 0xce, 0x48, 0x2d, 0x52,
	0xd2, 0xe3, 0x12, 0x74, 0x4f, 0x2d, 0x09, 0x01, 0xab, 0x29, 0x0e, 0x4a, 0x2d, 0x2c, 0x4d, 0x2d,
	0x2e, 0x11, 0x92, 0xe4, 0xe2, 0xc8, 0xcb, 0x4f, 0x49, 0x8d, 0xcf, 0x4e, 0xad, 0x94, 0x60, 0x54,
	0x60, 0xd4, 0xe0, 0x0c, 0x62, 0x07, 0xf1, 0xbd, 0x53, 0x2b, 0x95, 0xdc, 0xb9, 0x84, 0x90, 0xd5,
	0x17, 0x17, 0xe4, 0xe7, 0x15, 0xa7, 0x0a, 0x19, 0x72, 0xb1, 0x43, 0xac, 0x29, 0x96, 0x60, 0x54,
	0x60, 0xd6, 0xe0, 0x36, 0x12, 0xd7, 0x43, 0xb3, 0x48, 0x0f, 0xa2, 0x25, 0x08, 0xa6, 0x4e, 0x49,
	0x82, 0x8b, 0x0d, 0x22, 0x24, 0xc4, 0xc7, 0xc5, 0x94, 0x99, 0x02, 0xb5, 0x87, 0x29, 0x33, 0xc5,
	0x28, 0x8d, 0x8b, 0x3b, 0x10, 0xe4, 0x72, 0xa8, 0x74, 0x38, 0x17, 0x17, 0xc2, 0x46, 0x21, 0x25,
	0x0c, 0x83, 0x31, 0x9c, 0x2f, 0xa5, 0x8c, 0x57, 0x0d, 0xc4, 0xc9, 0x4a, 0x0c, 0x4e, 0xbc, 0x51,
	0xdc, 0xe0, 0x10, 0x82, 0xb8, 0x28, 0x89, 0x0d, 0x1c, 0x42, 0xc6, 0x80, 0x00, 0x00, 0x00, 0xff,
	0xff, 0x9a, 0xbe, 0xaa, 0xf8, 0x37, 0x01, 0x00, 0x00,
}
