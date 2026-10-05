package mlpb

import (
	"context"

	"google.golang.org/grpc"
)

// gRPC Client API for MLService

type MLServiceClient interface {
	InitializeRAG(ctx context.Context, in *InitializeRAGRequest, opts ...grpc.CallOption) (*InitializeRAGResponse, error)
	SearchRAG(ctx context.Context, in *SearchRAGRequest, opts ...grpc.CallOption) (*SearchRAGResponse, error)
	IndexRAGDynamic(ctx context.Context, in *IndexRAGDynamicRequest, opts ...grpc.CallOption) (*IndexRAGDynamicResponse, error)
	PersistRAG(ctx context.Context, in *PersistRAGRequest, opts ...grpc.CallOption) (*PersistRAGResponse, error)
	DeleteRAGSession(ctx context.Context, in *DeleteRAGSessionRequest, opts ...grpc.CallOption) (*DeleteRAGSessionResponse, error)
	ClassifyAudioEmotion(ctx context.Context, in *ClassifyAudioEmotionRequest, opts ...grpc.CallOption) (*EmotionResponse, error)
	ClassifyTextEmotion(ctx context.Context, in *ClassifyTextEmotionRequest, opts ...grpc.CallOption) (*EmotionResponse, error)
}

type mLServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewMLServiceClient(cc grpc.ClientConnInterface) MLServiceClient {
	return &mLServiceClient{cc}
}

func (c *mLServiceClient) InitializeRAG(ctx context.Context, in *InitializeRAGRequest, opts ...grpc.CallOption) (*InitializeRAGResponse, error) {
	out := new(InitializeRAGResponse)
	err := c.cc.Invoke(ctx, "/ml.MLService/InitializeRAG", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *mLServiceClient) SearchRAG(ctx context.Context, in *SearchRAGRequest, opts ...grpc.CallOption) (*SearchRAGResponse, error) {
	out := new(SearchRAGResponse)
	err := c.cc.Invoke(ctx, "/ml.MLService/SearchRAG", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *mLServiceClient) IndexRAGDynamic(ctx context.Context, in *IndexRAGDynamicRequest, opts ...grpc.CallOption) (*IndexRAGDynamicResponse, error) {
	out := new(IndexRAGDynamicResponse)
	err := c.cc.Invoke(ctx, "/ml.MLService/IndexRAGDynamic", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *mLServiceClient) PersistRAG(ctx context.Context, in *PersistRAGRequest, opts ...grpc.CallOption) (*PersistRAGResponse, error) {
	out := new(PersistRAGResponse)
	err := c.cc.Invoke(ctx, "/ml.MLService/PersistRAG", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *mLServiceClient) DeleteRAGSession(ctx context.Context, in *DeleteRAGSessionRequest, opts ...grpc.CallOption) (*DeleteRAGSessionResponse, error) {
	out := new(DeleteRAGSessionResponse)
	err := c.cc.Invoke(ctx, "/ml.MLService/DeleteRAGSession", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *mLServiceClient) ClassifyAudioEmotion(ctx context.Context, in *ClassifyAudioEmotionRequest, opts ...grpc.CallOption) (*EmotionResponse, error) {
	out := new(EmotionResponse)
	err := c.cc.Invoke(ctx, "/ml.MLService/ClassifyAudioEmotion", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *mLServiceClient) ClassifyTextEmotion(ctx context.Context, in *ClassifyTextEmotionRequest, opts ...grpc.CallOption) (*EmotionResponse, error) {
	out := new(EmotionResponse)
	err := c.cc.Invoke(ctx, "/ml.MLService/ClassifyTextEmotion", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Protobuf Struct Definitions & Message implementation

type InitializeRAGRequest struct {
	SessionId   string `protobuf:"bytes,1,opt,name=session_id,json=sessionId,proto3" json:"session_id,omitempty"`
	CompanyName string `protobuf:"bytes,2,opt,name=company_name,json=companyName,proto3" json:"company_name,omitempty"`
	TenantId    string `protobuf:"bytes,3,opt,name=tenant_id,json=tenantId,proto3" json:"tenant_id,omitempty"`
	Phone       string `protobuf:"bytes,4,opt,name=phone,proto3" json:"phone,omitempty"`
}

func (*InitializeRAGRequest) ProtoMessage() {}

type InitializeRAGResponse struct {
	Success bool   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	Message string `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
}

func (*InitializeRAGResponse) ProtoMessage() {}

type SearchRAGRequest struct {
	SessionId string `protobuf:"bytes,1,opt,name=session_id,json=sessionId,proto3" json:"session_id,omitempty"`
	Query     string `protobuf:"bytes,2,opt,name=query,proto3" json:"query,omitempty"`
	TenantId  string `protobuf:"bytes,3,opt,name=tenant_id,json=tenantId,proto3" json:"tenant_id,omitempty"`
}

func (*SearchRAGRequest) ProtoMessage() {}

type RAGResult struct {
	Text   string  `protobuf:"bytes,1,opt,name=text,proto3" json:"text,omitempty"`
	Score  float32 `protobuf:"fixed32,2,opt,name=score,proto3" json:"score,omitempty"`
	Source string  `protobuf:"bytes,3,opt,name=source,proto3" json:"source,omitempty"`
}

func (*RAGResult) ProtoMessage() {}

type SearchRAGResponse struct {
	Results []*RAGResult `protobuf:"bytes,1,rep,name=results,proto3" json:"results,omitempty"`
}

func (*SearchRAGResponse) ProtoMessage() {}

type IndexRAGDynamicRequest struct {
	SessionId string `protobuf:"bytes,1,opt,name=session_id,json=sessionId,proto3" json:"session_id,omitempty"`
	Text      string `protobuf:"bytes,2,opt,name=text,proto3" json:"text,omitempty"`
	Source    string `protobuf:"bytes,3,opt,name=source,proto3" json:"source,omitempty"`
	TenantId  string `protobuf:"bytes,4,opt,name=tenant_id,json=tenantId,proto3" json:"tenant_id,omitempty"`
}

func (*IndexRAGDynamicRequest) ProtoMessage() {}

type IndexRAGDynamicResponse struct {
	Success bool `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
}

func (*IndexRAGDynamicResponse) ProtoMessage() {}

type PersistRAGRequest struct {
	SessionId    string   `protobuf:"bytes,1,opt,name=session_id,json=sessionId,proto3" json:"session_id,omitempty"`
	Phone        string   `protobuf:"bytes,2,opt,name=phone,proto3" json:"phone,omitempty"`
	TenantId     string   `protobuf:"bytes,3,opt,name=tenant_id,json=tenantId,proto3" json:"tenant_id,omitempty"`
	Memories     []string `protobuf:"bytes,4,rep,name=memories,proto3" json:"memories,omitempty"`
	SourceCallId string   `protobuf:"bytes,5,opt,name=source_call_id,json=sourceCallId,proto3" json:"source_call_id,omitempty"`
}

func (*PersistRAGRequest) ProtoMessage() {}

type PersistRAGResponse struct {
	Success bool `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
}

func (*PersistRAGResponse) ProtoMessage() {}

type DeleteRAGSessionRequest struct {
	SessionId string `protobuf:"bytes,1,opt,name=session_id,json=sessionId,proto3" json:"session_id,omitempty"`
}

func (*DeleteRAGSessionRequest) ProtoMessage() {}

type DeleteRAGSessionResponse struct {
	Success bool `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
}

func (*DeleteRAGSessionResponse) ProtoMessage() {}

type ClassifyAudioEmotionRequest struct {
	AudioData []byte `protobuf:"bytes,1,opt,name=audio_data,json=audioData,proto3" json:"audio_data,omitempty"`
}

func (*ClassifyAudioEmotionRequest) ProtoMessage() {}

type ClassifyTextEmotionRequest struct {
	Text string `protobuf:"bytes,1,opt,name=text,proto3" json:"text,omitempty"`
}

func (*ClassifyTextEmotionRequest) ProtoMessage() {}

type EmotionResponse struct {
	Anger   float32 `protobuf:"fixed32,1,opt,name=anger,proto3" json:"anger,omitempty"`
	Joy     float32 `protobuf:"fixed32,2,opt,name=joy,proto3" json:"joy,omitempty"`
	Sadness float32 `protobuf:"fixed32,3,opt,name=sadness,proto3" json:"sadness,omitempty"`
	Neutral float32 `protobuf:"fixed32,4,opt,name=neutral,proto3" json:"neutral,omitempty"`
}

func (*EmotionResponse) ProtoMessage() {}
