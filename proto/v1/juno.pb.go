// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        v4.24.4
// source: proto/v1/juno.proto

package v1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Request message for the Jupiter.Lock rpc.
type LockRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Required during the initial call. The name of the lock.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Optional. Lease time in seconds. Default is 30s. Minimum is 5s.
	LeaseTime int64 `protobuf:"varint,2,opt,name=leaseTime,proto3" json:"leaseTime,omitempty"`
	// Optional. Set to true if you want to release the lock before its expiration period. Prioritized over `extend` when both are set to true.
	Release bool `protobuf:"varint,3,opt,name=release,proto3" json:"release,omitempty"`
	// Optional. Set to true if you want to extend the leased lock.
	Extend bool `protobuf:"varint,4,opt,name=extend,proto3" json:"extend,omitempty"`
	// Optional. Must be set to the received token (when the lock was acquired) when either `extend` or `release` is true.
	Token string `protobuf:"bytes,5,opt,name=token,proto3" json:"token,omitempty"`
	// Optional. If true, the API will block until the requested lock is released or has expired, giving the chance to attempt to reacquire the lock.
	WaitOnFail bool `protobuf:"varint,6,opt,name=waitOnFail,proto3" json:"waitOnFail,omitempty"`
}

func (x *LockRequest) Reset() {
	*x = LockRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_v1_juno_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LockRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LockRequest) ProtoMessage() {}

func (x *LockRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_v1_juno_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LockRequest.ProtoReflect.Descriptor instead.
func (*LockRequest) Descriptor() ([]byte, []int) {
	return file_proto_v1_juno_proto_rawDescGZIP(), []int{0}
}

func (x *LockRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *LockRequest) GetLeaseTime() int64 {
	if x != nil {
		return x.LeaseTime
	}
	return 0
}

func (x *LockRequest) GetRelease() bool {
	if x != nil {
		return x.Release
	}
	return false
}

func (x *LockRequest) GetExtend() bool {
	if x != nil {
		return x.Extend
	}
	return false
}

func (x *LockRequest) GetToken() string {
	if x != nil {
		return x.Token
	}
	return ""
}

func (x *LockRequest) GetWaitOnFail() bool {
	if x != nil {
		return x.WaitOnFail
	}
	return false
}

// Response message for the Jupiter.Lock rpc.
type LockResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The lock name used in the request payload.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// The lock token. Needed when extending the lock lease.
	Token string `protobuf:"bytes,2,opt,name=token,proto3" json:"token,omitempty"`
	// A general progress/activity indicator.
	Heartbeat int64 `protobuf:"varint,3,opt,name=heartbeat,proto3" json:"heartbeat,omitempty"`
}

func (x *LockResponse) Reset() {
	*x = LockResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_v1_juno_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LockResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LockResponse) ProtoMessage() {}

func (x *LockResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_v1_juno_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LockResponse.ProtoReflect.Descriptor instead.
func (*LockResponse) Descriptor() ([]byte, []int) {
	return file_proto_v1_juno_proto_rawDescGZIP(), []int{1}
}

func (x *LockResponse) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *LockResponse) GetToken() string {
	if x != nil {
		return x.Token
	}
	return ""
}

func (x *LockResponse) GetHeartbeat() int64 {
	if x != nil {
		return x.Heartbeat
	}
	return 0
}

// Request message for the Jupiter.AddToSet rpc.
type AddToSetRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Required. The key of the set.
	Key string `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	// Required. The value to add to the set specified in `key`.
	Value string `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
}

func (x *AddToSetRequest) Reset() {
	*x = AddToSetRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_v1_juno_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AddToSetRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AddToSetRequest) ProtoMessage() {}

func (x *AddToSetRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_v1_juno_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AddToSetRequest.ProtoReflect.Descriptor instead.
func (*AddToSetRequest) Descriptor() ([]byte, []int) {
	return file_proto_v1_juno_proto_rawDescGZIP(), []int{2}
}

func (x *AddToSetRequest) GetKey() string {
	if x != nil {
		return x.Key
	}
	return ""
}

func (x *AddToSetRequest) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

// Response message for the Jupiter.AddToSet rpc.
type AddToSetResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The updated key.
	Key string `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	// The number of items within the set after add.
	Count int64 `protobuf:"zigzag64,2,opt,name=count,proto3" json:"count,omitempty"`
}

func (x *AddToSetResponse) Reset() {
	*x = AddToSetResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_v1_juno_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AddToSetResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AddToSetResponse) ProtoMessage() {}

func (x *AddToSetResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_v1_juno_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AddToSetResponse.ProtoReflect.Descriptor instead.
func (*AddToSetResponse) Descriptor() ([]byte, []int) {
	return file_proto_v1_juno_proto_rawDescGZIP(), []int{3}
}

func (x *AddToSetResponse) GetKey() string {
	if x != nil {
		return x.Key
	}
	return ""
}

func (x *AddToSetResponse) GetCount() int64 {
	if x != nil {
		return x.Count
	}
	return 0
}

var File_proto_v1_juno_proto protoreflect.FileDescriptor

var file_proto_v1_juno_proto_rawDesc = []byte{
	0x0a, 0x13, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x6a, 0x75, 0x6e, 0x6f, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0d, 0x6a, 0x75, 0x6e, 0x6f, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x2e, 0x76, 0x31, 0x22, 0xa7, 0x01, 0x0a, 0x0b, 0x4c, 0x6f, 0x63, 0x6b, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x1c, 0x0a, 0x09, 0x6c, 0x65, 0x61, 0x73,
	0x65, 0x54, 0x69, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52, 0x09, 0x6c, 0x65, 0x61,
	0x73, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x72, 0x65, 0x6c, 0x65, 0x61, 0x73,
	0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x72, 0x65, 0x6c, 0x65, 0x61, 0x73, 0x65,
	0x12, 0x16, 0x0a, 0x06, 0x65, 0x78, 0x74, 0x65, 0x6e, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x08,
	0x52, 0x06, 0x65, 0x78, 0x74, 0x65, 0x6e, 0x64, 0x12, 0x14, 0x0a, 0x05, 0x74, 0x6f, 0x6b, 0x65,
	0x6e, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x1e,
	0x0a, 0x0a, 0x77, 0x61, 0x69, 0x74, 0x4f, 0x6e, 0x46, 0x61, 0x69, 0x6c, 0x18, 0x06, 0x20, 0x01,
	0x28, 0x08, 0x52, 0x0a, 0x77, 0x61, 0x69, 0x74, 0x4f, 0x6e, 0x46, 0x61, 0x69, 0x6c, 0x22, 0x56,
	0x0a, 0x0c, 0x4c, 0x6f, 0x63, 0x6b, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x12,
	0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61,
	0x6d, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x1c, 0x0a, 0x09, 0x68, 0x65, 0x61, 0x72,
	0x74, 0x62, 0x65, 0x61, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x09, 0x68, 0x65, 0x61,
	0x72, 0x74, 0x62, 0x65, 0x61, 0x74, 0x22, 0x39, 0x0a, 0x0f, 0x41, 0x64, 0x64, 0x54, 0x6f, 0x53,
	0x65, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76,
	0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75,
	0x65, 0x22, 0x3a, 0x0a, 0x10, 0x41, 0x64, 0x64, 0x54, 0x6f, 0x53, 0x65, 0x74, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x63, 0x6f, 0x75, 0x6e, 0x74,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x12, 0x52, 0x05, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x32, 0x98, 0x01,
	0x0a, 0x04, 0x4a, 0x75, 0x6e, 0x6f, 0x12, 0x43, 0x0a, 0x04, 0x4c, 0x6f, 0x63, 0x6b, 0x12, 0x1a,
	0x2e, 0x6a, 0x75, 0x6e, 0x6f, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x76, 0x31, 0x2e, 0x4c,
	0x6f, 0x63, 0x6b, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1b, 0x2e, 0x6a, 0x75, 0x6e,
	0x6f, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x76, 0x31, 0x2e, 0x4c, 0x6f, 0x63, 0x6b, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x28, 0x01, 0x30, 0x01, 0x12, 0x4b, 0x0a, 0x08, 0x41,
	0x64, 0x64, 0x54, 0x6f, 0x53, 0x65, 0x74, 0x12, 0x1e, 0x2e, 0x6a, 0x75, 0x6e, 0x6f, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x76, 0x31, 0x2e, 0x41, 0x64, 0x64, 0x54, 0x6f, 0x53, 0x65, 0x74,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1f, 0x2e, 0x6a, 0x75, 0x6e, 0x6f, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x76, 0x31, 0x2e, 0x41, 0x64, 0x64, 0x54, 0x6f, 0x53, 0x65, 0x74,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42, 0x20, 0x5a, 0x1e, 0x67, 0x69, 0x74, 0x68,
	0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x75, 0x73, 0x6c, 0x61,
	0x62, 0x73, 0x2f, 0x6a, 0x75, 0x6e, 0x6f, 0x2f, 0x76, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
}

var (
	file_proto_v1_juno_proto_rawDescOnce sync.Once
	file_proto_v1_juno_proto_rawDescData = file_proto_v1_juno_proto_rawDesc
)

func file_proto_v1_juno_proto_rawDescGZIP() []byte {
	file_proto_v1_juno_proto_rawDescOnce.Do(func() {
		file_proto_v1_juno_proto_rawDescData = protoimpl.X.CompressGZIP(file_proto_v1_juno_proto_rawDescData)
	})
	return file_proto_v1_juno_proto_rawDescData
}

var file_proto_v1_juno_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_proto_v1_juno_proto_goTypes = []interface{}{
	(*LockRequest)(nil),      // 0: juno.proto.v1.LockRequest
	(*LockResponse)(nil),     // 1: juno.proto.v1.LockResponse
	(*AddToSetRequest)(nil),  // 2: juno.proto.v1.AddToSetRequest
	(*AddToSetResponse)(nil), // 3: juno.proto.v1.AddToSetResponse
}
var file_proto_v1_juno_proto_depIdxs = []int32{
	0, // 0: juno.proto.v1.Juno.Lock:input_type -> juno.proto.v1.LockRequest
	2, // 1: juno.proto.v1.Juno.AddToSet:input_type -> juno.proto.v1.AddToSetRequest
	1, // 2: juno.proto.v1.Juno.Lock:output_type -> juno.proto.v1.LockResponse
	3, // 3: juno.proto.v1.Juno.AddToSet:output_type -> juno.proto.v1.AddToSetResponse
	2, // [2:4] is the sub-list for method output_type
	0, // [0:2] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_proto_v1_juno_proto_init() }
func file_proto_v1_juno_proto_init() {
	if File_proto_v1_juno_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_proto_v1_juno_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*LockRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_v1_juno_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*LockResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_v1_juno_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AddToSetRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_v1_juno_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AddToSetResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_proto_v1_juno_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_proto_v1_juno_proto_goTypes,
		DependencyIndexes: file_proto_v1_juno_proto_depIdxs,
		MessageInfos:      file_proto_v1_juno_proto_msgTypes,
	}.Build()
	File_proto_v1_juno_proto = out.File
	file_proto_v1_juno_proto_rawDesc = nil
	file_proto_v1_juno_proto_goTypes = nil
	file_proto_v1_juno_proto_depIdxs = nil
}
