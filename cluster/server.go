package cluster

import (
	"game/component"
	"game/internal/protos"
	"game/session"
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
	Frontend      bool
}

type Server struct {
	Options
	rpcDiscoveryServer *grpcDiscoveryServer
	rpcDiscoveryClient *grpcDiscoveryClient
	RemoteServer       *RemoteServer
	rpcClient          *rpcClient
	server             *grpc.Server
	ServerAddr         string
	services           map[string]component.Component
	sessionPool        session.SessionPool
	handlerServer      *handlerServer
}

func (s *Server) Startup() error {
	s.rpcClient = newRPCClient()
	s.rpcDiscoveryClient = NewGRPCDiscoveryClient(s.rpcClient)
	s.sessionPool = session.NewSessionPool()

	if err := s.initNode(); err != nil {
		return err
	}
	s.initFrontend()
	return nil
}

func (s *Server) initNode() error {
	ls, err := net.Listen("tcp", s.ServerAddr)
	if err != nil {
		return err
	}

	s.server = grpc.NewServer()
	s.RemoteServer = newRemoteServer(s.rpcDiscoveryClient, s.sessionPool)
	protos.RegisterMembersServerServer(s.server, s.rpcDiscoveryClient)
	protos.RegisterRemoteServerServer(s.server, s.RemoteServer)

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

	return s.rpcDiscoveryClient.register(s.Frontend, s.IsMaster, s.Name, s.ServerAddr, s.AdvertiseAddr)
}

func (s *Server) initFrontend() {
	if s.ClientAddr == "" {
		return
	}
	s.handlerServer = newHandlerServer(s.sessionPool, s.services, s.rpcDiscoveryClient)
	listener, err := net.Listen("tcp", s.ClientAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Println(err)
				continue
			}
			go s.handlerServer.handler(conn)
		}
	}()
}

func (s *Server) Register(comp component.Component) {
	if s.services == nil {
		s.services = map[string]component.Component{}
	}
	sv := component.NewService(comp, s.sessionPool)
	sv.ExtractHandler()
	s.services[sv.Name] = comp
	s.RemoteServer.services[sv.Name] = sv
}

func (s *Server) Shutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c
	if s.rpcDiscoveryClient != nil {
		s.rpcDiscoveryClient.Shutdown()
	}
	log.Println("Shutdown 1")
	if s.rpcDiscoveryServer != nil {
		s.rpcDiscoveryServer.Shutdown()
	}
	s.rpcClient.closePool()
	log.Println("Shutdown 2")
	if s.RemoteServer != nil {
		s.RemoteServer.Close()
	}
	log.Println("Shutdown 3")
	s.server.GracefulStop()
	log.Println("Shutdown 4")
}
