package handler

import (
	"fmt"
	"game/agent"
	"game/component"
	"game/internal/codec"
	"game/internal/message"
	"game/internal/packet"
	"game/session"
	"log"
	"net"
	"strings"
)

type AgentHandler struct {
	Name        string
	decode      *codec.Decoder
	sessionPool session.SessionPool
	remote      *RemoteHandler
	components  *component.Components
}

func NewAgentHandler(name string, sessionPool session.SessionPool, remote *RemoteHandler, components *component.Components) *AgentHandler {
	return &AgentHandler{
		Name:        name,
		decode:      codec.NewDecoder(),
		sessionPool: sessionPool,
		components:  components,
	}
}

func (h *AgentHandler) Handler(conn net.Conn) {
	a := agent.NewAgent(conn, h.sessionPool, nil)

	defer func() {
		a.Close()
		h.components.OnSessionDisconnect(a.Session())
		h.remote.notifyRemoteOnSessionClose(a.Session())
	}()
	h.components.OnSessionConnect(a.Session())

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
		h.processMessage(a, message.Decode(pkg.Data))

	default:
		return fmt.Errorf("[AgentHandler/ProcessPacket] packet type[%d] not found", pkg.Type)
	}
	return nil
}

func (h *AgentHandler) processMessage(a *agent.Agent, msg *message.Message) error {
	index := strings.Index(msg.Route, ".")
	if index != 3 {
		return fmt.Errorf("[AgentHandler/ProcessMessage] route[%v] error", msg.Route)
	}
	if h.Name == msg.Route[:index] {
		h.components.Tell(h.Name, msg)
	} else {
		h.remote.remoteProcess(a.Session(), h.Name, msg)
	}
	return nil
}
