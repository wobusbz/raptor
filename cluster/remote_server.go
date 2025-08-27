package cluster

import (
	"fmt"
	"game/component"
	"game/internal/message"
	"game/internal/protos"
	"game/session"
	"log"
	"strings"
	"sync/atomic"

	"google.golang.org/grpc"
)

type RemoteServer struct {
	protos.UnimplementedRemoteServerServer
	grpcDiscoveryClient *grpcDiscoveryClient
	services            map[string]*component.Service
	RemoteStream        grpc.ClientStreamingClient[protos.RemoteMessage, protos.RemoteMessage]
	sessionPool         session.SessionPool
	stopc               atomic.Bool
}

func newRemoteServer(grpcDiscoveryClient *grpcDiscoveryClient, sessionPool session.SessionPool) *RemoteServer {
	return &RemoteServer{
		grpcDiscoveryClient: grpcDiscoveryClient,
		services:            map[string]*component.Service{},
		sessionPool:         sessionPool,
	}
}

func (r *RemoteServer) Receive(stream grpc.ClientStreamingServer[protos.RemoteMessage, protos.RemoteMessage]) error {
	for {
		recv, err := stream.Recv()
		if err != nil {
			return err
		}
		if r.stopc.Load() {
			return nil
		}
		if err = r.dispatch(recv); err != nil {
			log.Println(err)
		}
	}
}

func (r *RemoteServer) dispatch(recv *protos.RemoteMessage) error {
	switch recv.GetKind() {
	case protos.RemoteMessage_KIND_RPC:
		routes := strings.Split(recv.RPCMessage.GetRoute(), "/")
		if len(routes) < 2 {
			return fmt.Errorf("[RemoteMessage/Receive] Route[%s] length < 2 \n", recv.RPCMessage.GetRoute())
		}
		server, ok := r.services[routes[1]]
		if !ok {
			return fmt.Errorf("[RemoteMessage/Receive] server[%s] not found \n", routes[1])
		}
		err := server.Tell(&message.Message{ID: uint(recv.RPCMessage.GetSessionID()), Route: routes[2], Data: recv.RPCMessage.GetData()})
		if err != nil {
			return fmt.Errorf("[RemoteMessage/Receive] Tell %v \n", err)
		}
	case protos.RemoteMessage_KIND_NOTIFY: // 向某一组服务器进行广播，如果是Gate就是向所有的用户广播，其它则就是向所有的服务进行广播

	case protos.RemoteMessage_KIND_PUSH: // 向网关推送指定session
		session, ok := r.sessionPool.GetSessionByID(recv.PushMessage.GetSessionID())
		if !ok {
			return fmt.Errorf("[RemoteServer/dispatch] session %d not found", recv.PushMessage.GetSessionID())
		}
		if err := session.Push(recv.PushMessage); err != nil {
			return fmt.Errorf("[RemoteMessage/Receive] session Push %s \n", err)
		}
	case protos.RemoteMessage_KIND_ON_SESSION_BIND_UID:
	case protos.RemoteMessage_KIND_ON_SESSION_CONNECT:
		session, ok := r.sessionPool.GetSessionByID(recv.OnSessionConnectMessage.GetID())
		if !ok {
			session = r.sessionPool.OnConnectionNewSession(r, recv.OnSessionConnectMessage.GetID())
		}
		for kname, instances := range recv.OnSessionConnectMessage.Instances {
			if _, ok := session.FindRoutes(kname); !ok {
				entity, ok := r.grpcDiscoveryClient.GetRemoteClient(instances.ServiceName, instances.Addr)
				if !ok {
					continue
				}
				session.BindServer(kname, entity)
			}
		}
	case protos.RemoteMessage_KIND_ON_SESSION_DISCONNECT:
		log.Println(recv.OnSessionDisconnectionMessage.GetID())
	default:
	}
	return nil
}

func (r *RemoteServer) Push(sessionId int64, route string, data []byte) error {
	session, ok := r.sessionPool.GetSessionByID(sessionId)
	if !ok {
		return fmt.Errorf("[RemoteServer/Push] Session %d not found", sessionId)
	}
	entity, ok := session.FindRoutes("GATE")
	if !ok {
		return fmt.Errorf("[RemoteServer/Push] NetworkEntity %s not found", "GATE")
	}
	return entity.Push(sessionId, route, data)
}

func (r *RemoteServer) RPC(sessionId int64, route string, data []byte) error {
	session, ok := r.sessionPool.GetSessionByID(sessionId)
	if !ok {
		return fmt.Errorf("[RemoteServer/RPC] Session %d not found", sessionId)
	}
	routes := strings.Split(route, "/")
	entity, ok := session.FindRoutes(routes[0])
	if !ok {
		return fmt.Errorf("[RemoteServer/Push] NetworkEntity %s not found", "GATE")
	}
	return entity.RPC(sessionId, route, data)
}

func (r *RemoteServer) Close() error {
	r.stopc.Store(true)
	if r.RemoteStream != nil {
		return r.RemoteStream.CloseSend()
	}
	return nil
}
