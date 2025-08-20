package cluster

import (
	"game/internal/protos"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
)

type Options struct {
	Name          string
	IsMaster      bool
	AdvertiseAddr string
	ClientAddr    string
}

type Server struct {
	Options
	rpcDiscoveryServer *grpcDiscoveryServer
	rpcDiscoveryClient *grpcDiscoveryClient
	rpcRemoteClient    *grpcRemoteClient
	server             *grpc.Server
	ServerAddr         string
	rpcClient          *rpcClient
}

func (s *Server) Startup() error {
	return s.initNode()
}

func (s *Server) initNode() error {
	ls, err := net.Listen("tcp", s.ServerAddr)
	if err != nil {
		return err
	}

	s.server = grpc.NewServer()
	s.rpcClient = newRPCClient()
	s.rpcRemoteClient = newGRPCRemoteClient()
	s.rpcDiscoveryClient = NewGRPCDiscoveryClient(s.rpcClient, s.rpcRemoteClient)
	protos.RegisterMembersServerServer(s.server, s.rpcDiscoveryClient)
	protos.RegisterRemoteServerServer(s.server, newRemoteServer())

	if s.IsMaster {
		s.rpcDiscoveryServer = NewGRPCDiscoveryServer(s.rpcDiscoveryClient, s.rpcClient)
		protos.RegisterRegistryServerServer(s.server, s.rpcDiscoveryServer)
		s.rpcDiscoveryClient.setGRPCDiscoveryServer(s.rpcDiscoveryServer)
	}

	go func() {
		if err = s.server.Serve(ls); err != nil {
			log.Fatalf("Start current node failed: %v", err)
		}
	}()

	return s.rpcDiscoveryClient.register(s.IsMaster, s.Name, s.ServerAddr, s.AdvertiseAddr)
}

func (s *Server) Shutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c
	s.server.GracefulStop()
	if s.rpcDiscoveryClient != nil {
		s.rpcDiscoveryClient.Shutdown()
	}
	if s.rpcDiscoveryServer != nil {
		s.rpcDiscoveryServer.Shutdown()
	}
	s.rpcClient.closePool()
	if s.rpcRemoteClient != nil {
		s.rpcRemoteClient.Shutdown()
	}
}
