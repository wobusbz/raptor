package handler

import (
	"fmt"
	"game/agent"
	"game/cluster"
	"game/component"
	"game/internal/codec"
	"game/internal/message"
	"game/internal/packet"
	"game/session"
	"game/stream"
	"log"
	"net"
	"runtime/debug"
)

type AgentHandler struct {
	server      cluster.ServiceDiscovery
	rpcClient   *stream.StreamClientManager
	decode      *codec.Decoder
	sessionPool session.SessionPool
	remote      *RemoteHandler
	components  *component.Components
}

func NewAgentHandler(
	server cluster.ServiceDiscovery,
	rpcClient *stream.StreamClientManager,
	sessionPool session.SessionPool,
	remote *RemoteHandler,
	components *component.Components,
) *AgentHandler {
	return &AgentHandler{
		server:      server,
		rpcClient:   rpcClient,
		decode:      codec.NewDecoder(),
		sessionPool: sessionPool,
		remote:      remote,
		components:  components,
	}
}

func (h *AgentHandler) Handler(conn net.Conn) {
	a := agent.NewAgent(conn, h.sessionPool, nil)

	defer func() {
		if r := recover(); r != nil {
			log.Printf("%s\n%v\n", string(debug.Stack()), r)
		}
		a.Close()
		h.components.OnSessionDisconnect(a.Session())
		h.server.NotifyOnSessionClose(a.Session())
		h.sessionPool.DelSessionByID(a.Session().ID())
	}()
	h.components.OnSessionConnect(a.Session())
	if err := h.server.NotifyOnSession(a.Session()); err != nil {
		log.Println(err)
		return
	}
	var b = make([]byte, 2048)
	for {
		n, err := conn.Read(b)
		if err != nil {
			log.Printf("[AgentHandler/Handler] network Read %v failed \n", err)
			return
		}
		packeks, err := h.decode.Decode(b[:n])
		if err != nil {
			log.Printf("[AgentHandler/Handler] package devode %v failed \n", err)
			return
		}
		if len(packeks) < 1 {
			log.Printf("[AgentHandler/Handler] empty package \n")
			return
		}
		a.UpdateHeartbeat()
		for _, pkg := range packeks {
			err = h.processPacket(a, pkg)
			if err != nil {
				log.Printf("[AgentHandler/Handler] processPacket %v \n", err)
				return
			}
		}
	}
}

func (h *AgentHandler) processPacket(a *agent.Agent, pkg *packet.Packet) error {
	switch pkg.Type {
	case packet.Heartbeat:
	case packet.Forward:
	case packet.Data:
		return h.processMessage(a, message.Decode(pkg.Data))
	default:
		return fmt.Errorf("[AgentHandler/ProcessPacket] packet type[%d] not found", pkg.Type)
	}
	return nil
}

func (h *AgentHandler) processMessage(a *agent.Agent, msg *message.Message) error {
	if h.components.HasMessageID(uint32(msg.ID)) {
		return h.components.Tell(h.server.Name(), msg)
	}
	return h.remote.remoteProcess(a.Session(), msg)
}
