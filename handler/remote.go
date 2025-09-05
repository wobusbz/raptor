package handler

import (
	"fmt"
	"game/agent"
	"game/cluster"
	"game/component"
	"game/internal/message"
	"game/internal/protos"
	"game/session"
	"game/stream"
	"log"
	"runtime/debug"
	"strings"

	"google.golang.org/grpc"
)

type RemoteHandler struct {
	protos.UnimplementedRemoteServerServer
	server      cluster.ServiceDiscovery
	sessionPool session.SessionPool
	rpcClient   *stream.StreamClientManager
	stream      grpc.ClientStreamingClient[protos.RemoteMessage, protos.RemoteMessage]
	components  *component.Components
}

func NewRemoteHandler(
	server cluster.ServiceDiscovery,
	sessionPool session.SessionPool,
	rpcClient *stream.StreamClientManager,
	components *component.Components,
) *RemoteHandler {
	return &RemoteHandler{server: server, sessionPool: sessionPool, rpcClient: rpcClient, components: components}
}

func (r *RemoteHandler) Receive(stream grpc.BidiStreamingServer[protos.RemoteMessage, protos.RemoteMessage]) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[RemoteHandler/Receive] %s\n%v\n", debug.Stack(), r)
		}
	}()
	for {
		recv, err := stream.Recv()
		if err != nil {
			log.Println(err)
			return err
		}
		r.processReceive(recv)
	}
}

func (r *RemoteHandler) processReceive(m *protos.RemoteMessage) {
	switch m.Kind {
	case protos.RemoteMessage_KIND_RPC:
		session, ok := r.sessionPool.GetSessionByID(m.GetSessionID())
		if !ok {
			return
		}
		if err := r.components.Route(session, &message.Message{Route: m.RPCMessage.Route, Data: m.RPCMessage.Data}); err != nil {
			log.Println(err)
		}
	case protos.RemoteMessage_KIND_NOTIFY:

	case protos.RemoteMessage_KIND_PUSH:
		session, ok := r.sessionPool.GetSessionByID(m.PushMessage.GetSessionID())
		if !ok {
			return
		}
		session.Response(int(m.PushMessage.GetMessageId()), m.PushMessage.GetData())

	case protos.RemoteMessage_KIND_ON_SESSION_BIND_UID:

	case protos.RemoteMessage_KIND_ON_SESSION_CONNECT:
		var session session.Session
		for _, v := range m.OnSessionConnectMessage.GetInstances() {
			if !v.Frontend {
				continue
			}
			gateClient, err := r.rpcClient.GetStreamClient(v.GetServiceName(), v.GetInstanceId(), v.GetAddr())
			if err != nil {
				log.Println(err)
				return
			}
			agent := agent.NewRemote(gateClient, r.sessionPool, r.remoteCall)
			session = r.sessionPool.NewSession(agent, m.OnSessionConnectMessage.GetID())
		}
		if session == nil {
			return
		}
		for _, v := range m.OnSessionConnectMessage.GetInstances() {
			session.Routers().BindServer(v.GetServiceName(), v.GetInstanceId())
		}
		r.components.OnSessionConnect(session)

	case protos.RemoteMessage_KIND_ON_SESSION_DISCONNECT:
		session, ok := r.sessionPool.GetSessionByID(m.GetSessionID())
		if !ok {
			return
		}
		r.sessionPool.DelSessionByID(m.OnSessionConnectMessage.GetID())
		r.components.OnSessionDisconnect(session)
	}
}

func (r *RemoteHandler) remoteCall(session session.Session, svrname string, msg *protos.RemoteMessage) error {
	instanceId, ok := session.Routers().FindServer(svrname)
	if !ok {
		return fmt.Errorf("[RemoteHandler/RemoteCall] service[%s] router not found", svrname)
	}
	svrNodeInfo, ok := r.server.GetNodeInfo(svrname, instanceId)
	if !ok {
		return fmt.Errorf("[RemoteHandler/notifyRemoteOnSessionClose] server %s not found", svrname)
	}
	rpcClient, err := r.rpcClient.GetStreamClient(svrname, instanceId, svrNodeInfo.Instances.Addr)
	if err != nil {
		return fmt.Errorf("[RemoteHandler/RemoteCall] RPCClient %w", err)
	}
	return rpcClient.Send(msg)
}

func (r *RemoteHandler) remoteProcess(session session.Session, msg *message.Message) error {
	var remoteMessage = &protos.RemoteMessage{SessionID: session.ID()}
	switch msg.Type {
	case message.Request:
		remoteMessage.Kind = protos.RemoteMessage_KIND_RPC
		remoteMessage.RPCMessage = &protos.RPCMessage{Data: msg.Data, Route: msg.Route}

	case message.Push:
		remoteMessage.Kind = protos.RemoteMessage_KIND_PUSH
		remoteMessage.PushMessage = &protos.PushMessage{Data: msg.Data, MessageId: int32(msg.ID)}

	default:
		return fmt.Errorf("[RemoteHandler/RemoteProcess] remoteProcess msg.type[%d] not found", msg.Type)
	}
	return r.remoteCall(session, strings.ToUpper(strings.Split(r.server.GetRoute(uint32(msg.ID)), ".")[0]), remoteMessage)
}
