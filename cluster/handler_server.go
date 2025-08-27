package cluster

import (
	"game/component"
	"game/internal/message"
	"game/session"
	"log"
	"net"
	"strings"
)

type handlerServer struct {
	sessionPool         session.SessionPool
	comps               map[string]component.Component
	grpcDiscoveryClient *grpcDiscoveryClient
}

func newHandlerServer(
	sessionPool session.SessionPool,
	comps map[string]component.Component,
	grpcDiscoveryClient *grpcDiscoveryClient,
) *handlerServer {

	return &handlerServer{
		sessionPool:         sessionPool,
		comps:               comps,
		grpcDiscoveryClient: grpcDiscoveryClient,
	}
}

func (h *handlerServer) handler(conn net.Conn) {
	agent := newAgent(conn, h.sessionPool, h.comps, h.grpcDiscoveryClient)
	defer func() {
		agent.Close()
	}()
	go agent.write()
	buf := make([]byte, 2048)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Println(err)
			return
		}
		packs, err := agent.pack.Decode(buf[:n])
		if err != nil {
			log.Println(err)
			return
		}
		for _, pack := range packs {
			h.processHandler(agent.session, message.Decode(pack.Data))
		}
	}
}

func (h *handlerServer) processHandler(session session.Session, message *message.Message) {
	routes := strings.Split(message.Route, "/")
	remoteClient, ok := session.FindRoutes(routes[0])
	if !ok {
		return
	}
	remoteClient.RPC(session.ID(), message.Route, message.Data)
}
