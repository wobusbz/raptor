package agent

import (
	"fmt"
	"game/internal/message"
	"game/internal/protos"
	"game/session"
	"game/stream"
	"sync/atomic"

	"google.golang.org/protobuf/proto"
)

type (
	remoteHandler func(session session.Session, svrname string, message *message.Message) error

	remote struct {
		gateClient    stream.StreamClient
		session       session.Session
		sessionPool   session.SessionPool
		remoteHandler remoteHandler
		chDie         chan struct{}
		state         atomic.Bool
	}
)

func NewRemote(gateClient stream.StreamClient, sessionPool session.SessionPool, remoteHandler remoteHandler) *remote {
	r := &remote{
		gateClient:    gateClient,
		sessionPool:   sessionPool,
		remoteHandler: remoteHandler,
		chDie:         make(chan struct{}),
	}
	r.session = r.sessionPool.NewSession(nil, 0)
	return r
}

func (r *remote) Push(pb proto.Message) error {
	pbdata, err := proto.Marshal(pb)
	if err != nil {
		return err
	}

	m := &protos.RemoteMessage{
		Kind:          protos.RemoteMessage_KIND_PUSH,
		NotifyMessage: &protos.NotifyMessage{SessionID: r.session.ID(), Data: pbdata},
	}

	return r.gateClient.Send(m)
}

func (r *remote) RPC(pb proto.Message) error {
	pber, ok := pb.(interface {
		GetRoute() string
		GetSvrName() string
	})
	if !ok {
		return fmt.Errorf("[Remote/RPC] Reflection GetSvrName failure")
	}
	pbdata, err := proto.Marshal(pb)
	if err != nil {
		return err
	}
	m := &protos.RemoteMessage{
		Kind:       protos.RemoteMessage_KIND_RPC,
		RPCMessage: &protos.RPCMessage{SessionID: r.session.ID(), Data: pbdata},
	}
	_ = m
	return r.remoteHandler(r.session, pber.GetSvrName(), nil)
}

func (r *remote) Close() error {
	return r.gateClient.Close()
}
