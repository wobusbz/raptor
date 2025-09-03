package handler

import (
	"errors"
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
	return &RemoteHandler{
		server:      server,
		sessionPool: sessionPool,
		rpcClient:   rpcClient,
		components:  components,
	}
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
			return err
		}
		r.processReceive(recv)
	}
}

func (r *RemoteHandler) processReceive(m *protos.RemoteMessage) {
	session, ok := r.sessionPool.GetSessionByID(m.PushMessage.GetSessionID())
	if !ok {
		return
	}
	switch m.Kind {
	case protos.RemoteMessage_KIND_RPC:
		r.remoteProcess(session, "", &message.Message{})
	case protos.RemoteMessage_KIND_NOTIFY:

	case protos.RemoteMessage_KIND_PUSH:
		session.Response(int(m.PushMessage.GetMessageId()), m.PushMessage.GetData())

	case protos.RemoteMessage_KIND_ON_SESSION_BIND_UID:

	case protos.RemoteMessage_KIND_ON_SESSION_CONNECT:
		agent := agent.NewRemote(nil, r.sessionPool, r.remoteProcess)
		r.sessionPool.NewSession(agent, m.OnSessionConnectMessage.GetID())
		r.components.OnSessionConnect(session)

	case protos.RemoteMessage_KIND_ON_SESSION_DISCONNECT:
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

func (r *RemoteHandler) remoteProcess(session session.Session, svrname string, msg *message.Message) error {
	var remoteMessage = &protos.RemoteMessage{SessionID: session.ID()}
	switch msg.Type {
	case message.Notify:
		remoteMessage.Kind = protos.RemoteMessage_KIND_RPC
		remoteMessage.NotifyMessage = &protos.NotifyMessage{Data: msg.Data, Route: msg.Route}

	case message.Push:
		remoteMessage.Kind = protos.RemoteMessage_KIND_PUSH
		remoteMessage.PushMessage = &protos.PushMessage{Data: msg.Data, MessageId: int32(msg.ID)}

	default:
		return fmt.Errorf("[RemoteHandler/RemoteProcess] remoteProcess msg.type[%d] not found", msg.Type)
	}
	return r.remoteCall(session, svrname, remoteMessage)
}

func (r *RemoteHandler) notifyRemoteOnSessionClose(session session.Session) error {
	var (
		errs          []error
		remoteMessage = &protos.RemoteMessage{
			Kind:      protos.RemoteMessage_KIND_ON_SESSION_DISCONNECT,
			SessionID: session.ID(),
		}
	)
	for svrname, instanceId := range session.Routers().Routers() {
		svrNodeInfo, ok := r.server.GetNodeInfo(svrname, instanceId)
		if !ok {
			errs = append(errs, fmt.Errorf("[RemoteHandler/notifyRemoteOnSessionClose] server %s not found", svrname))
			continue
		}
		rpcClient, err := r.rpcClient.GetStreamClient(svrname, instanceId, svrNodeInfo.Instances.Addr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		errs = append(errs, rpcClient.Send(remoteMessage))
	}
	return errors.Join(errs...)
}
