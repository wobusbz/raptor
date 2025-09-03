package cluster

import (
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
	IsMaster      bool
	AdvertiseAddr string
	ClientAddr    string
	Frontend      bool
}

type Server struct {
	Options
	server              *grpc.Server
	ServerAddr          string
	sessionPool         session.SessionPool
	streamClientManager *stream.StreamClientManager
	components          *component.Components
	agentHandler        *handler.AgentHandler
	remoteHandler       *handler.RemoteHandler
}

func (s *Server) Startup() error {
	s.sessionPool = session.NewSessionPool()
	s.streamClientManager = stream.NewStreamClientManager()
	s.components = component.NewComponents(s.sessionPool)

	s.remoteHandler = handler.NewRemoteHandler(nil, s.sessionPool, s.streamClientManager, s.components)
	s.agentHandler = handler.NewAgentHandler(s.Name, s.sessionPool, s.remoteHandler, s.components)

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
	if s.IsMaster {
		protos.RegisterRegistryServerServer(s.server, nil)
	}
	protos.RegisterRemoteServerServer(s.server, s.remoteHandler)
	go func() {
		if err = s.server.Serve(ls); err != nil {
			log.Fatalf("Start current node failed: %v", err)
		}
	}()
	return nil
}

func (s *Server) initFrontend() error {
	if s.ClientAddr == "" {
		return nil
	}
	ls, err := net.Listen("tcp", s.ServerAddr)
	if err != nil {
		return err
	}
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

func (s *Server) Register(comp component.Component) {
}

func (s *Server) Shutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c
	s.server.GracefulStop()
}
