package server

import (
	"errors"
	"game/cluster"
	"game/component"
	"game/handler"
	"game/internal/protos"
	"game/session"
	"game/stream"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
)

type Options struct {
	Name          string
	AdvertiseAddr string
	ClientAddr    string
	Frontend      bool
}

type Server struct {
	Options
	clientListener      net.Listener
	server              *grpc.Server
	ServerAddr          string
	discoverServer      *cluster.GrpcDiscoveryServer
	sessionPool         session.SessionPool
	streamClientManager *stream.StreamClientManager
	components          *component.Components
	agentHandler        *handler.AgentHandler
	remoteHandler       *handler.RemoteHandler
}

func (s *Server) Startup() error {
	s.streamClientManager = stream.NewStreamClientManager()
	s.discoverServer = cluster.NewGRPCDiscoveryServer(s.AdvertiseAddr, s.streamClientManager)
	s.sessionPool = session.NewSessionPool()
	s.components = component.NewComponents(s.sessionPool)

	s.remoteHandler = handler.NewRemoteHandler(s.discoverServer, s.sessionPool, s.streamClientManager, s.components)
	s.agentHandler = handler.NewAgentHandler(s.discoverServer, s.streamClientManager, s.sessionPool, s.remoteHandler, s.components)

	return errors.Join(s.initNode(), s.initFrontend())
}

func (s *Server) initNode() error {
	ls, err := net.Listen("tcp", s.ServerAddr)
	if err != nil {
		return err
	}
	s.server = grpc.NewServer()
	protos.RegisterRegistryServerServer(s.server, s.discoverServer)
	protos.RegisterRemoteServerServer(s.server, s.remoteHandler)
	go func() {
		if err = s.server.Serve(ls); err != nil {
			log.Fatalf("Start current node failed: %v", err)
		}
	}()
	return s.discoverServer.RegisterNode(s.Name, s.ServerAddr, s.ClientAddr != "", s.components.LocalHandler())
}

func (s *Server) initFrontend() error {
	if s.ClientAddr == "" {
		return nil
	}
	ls, err := net.Listen("tcp", s.ClientAddr)
	if err != nil {
		return err
	}
	s.clientListener = ls
	go func() {
		for {
			conn, err := ls.Accept()
			if err != nil {
				log.Printf("[Server/initFrontend] accept %v\n", err)
				return
			}
			go s.agentHandler.Handler(conn)
		}
	}()
	return nil
}

func (s *Server) Shutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c
	s.discoverServer.Shutdown()
	s.streamClientManager.Shutdown()
	if s.clientListener != nil {
		s.clientListener.Close()
	}
	s.server.Stop()
}
