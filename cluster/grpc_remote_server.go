package cluster

import (
	"context"
	"game/internal/protos"
)

type grpcRemoteServer struct {
	protos.UnimplementedRemoteServerServer
}

func newRemoteServer() *grpcRemoteServer {
	return &grpcRemoteServer{}
}

func (r *grpcRemoteServer) Call(ctx context.Context, in *protos.CallRequest) (*protos.CallResponse, error) {
	// TODO: 这里应路由到业务层，当前简单 echo
	return &protos.CallResponse{Data: in.GetData()}, nil
}

func (r *grpcRemoteServer) Notify(ctx context.Context, in *protos.NotifyRequest) (*protos.NotifyResponse, error) {
	// TODO: 业务层异步处理，当前直接成功
	return &protos.NotifyResponse{}, nil
}
