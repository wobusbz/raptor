package main

import (
	"context"
	"game/internal/protos"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
)

type grpcRemoteServer struct {
	protos.UnimplementedRemoteServerServer
}

func newRemoteServer() *grpcRemoteServer {
	return &grpcRemoteServer{}
}

func (r *grpcRemoteServer) Call(ctx context.Context, in *protos.CallRequest) (*protos.CallResponse, error) {
	log.Println(string(in.GetData()))
	return &protos.CallResponse{Data: in.GetData()}, nil
}

func (r *grpcRemoteServer) Notify(ctx context.Context, in *protos.NotifyRequest) (*protos.NotifyResponse, error) {
	return &protos.NotifyResponse{}, nil
}

func main() {
	ls, err := net.Listen("tcp", "0.0.0.0:8800")
	if err != nil {
		slog.Debug("[Test07] Listen failed", slog.Any("tcp", err))
		return
	}
	server := grpc.NewServer()
	protos.RegisterRemoteServerServer(server, newRemoteServer())
	go func() {
		server.Serve(ls)
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c
}
