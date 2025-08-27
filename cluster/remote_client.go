package cluster

import (
	"context"
	"errors"
	"game/internal/protos"
	"game/networkentity"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

var _ networkentity.NetworkEntity = (*RemoteClient)(nil)

type RemoteClient struct {
	conn      *grpc.ClientConn
	stream    grpc.ClientStreamingClient[protos.RemoteMessage, protos.RemoteMessage]
	addr      string
	connonce  sync.Once
	sendch    chan *protos.RemoteMessage
	stop      chan struct{}
	closeonce sync.Once
}

func newRemoteClient(addr string) *RemoteClient {
	r := &RemoteClient{addr: addr, stop: make(chan struct{}), sendch: make(chan *protos.RemoteMessage, 1024)}
	r.initializeInternal()
	return r
}

func (r *RemoteClient) initializeInternal() error {
	r.connonce.Do(func() {
		conn, err := grpc.NewClient(r.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Println(err)
			return
		}
		r.conn = conn
	})
	stream, err := protos.NewRemoteServerClient(r.conn).Receive(context.Background())
	if err != nil {
		return err
	}
	r.stream = stream
	go r.dispatch()
	return nil
}

func (r *RemoteClient) dispatch() {
	for {
		select {
		case pb := <-r.sendch:
			if err := r.stream.Send(pb); err != nil {
				log.Println(err)
			}
		case <-r.stop:
			return
		}
	}
}

func (r *RemoteClient) Push(sessionId int64, route string, data []byte) error {
	select {
	case r.sendch <- &protos.RemoteMessage{
		Kind:        protos.RemoteMessage_KIND_PUSH,
		PushMessage: &protos.PushMessage{SessionID: sessionId, Data: data},
	}:
	default:
		return errors.New("[RemoteClient/Push] sendch full")
	}
	return nil
}

func (r *RemoteClient) RPC(sessionId int64, route string, data []byte) error {
	select {
	case r.sendch <- &protos.RemoteMessage{
		Kind:       protos.RemoteMessage_KIND_RPC,
		RPCMessage: &protos.RPCMessage{SessionID: sessionId, Route: route, Data: data},
	}:
	default:
		return errors.New("[RemoteClient/RPC] sendch full")
	}
	return nil
}

func (r *RemoteClient) Response(modelName, method string, message proto.Message) error {
	return nil
}

func (r *RemoteClient) Close() error {
	r.closeonce.Do(func() {
		close(r.stop)
		r.stream.CloseSend()
	})
	return nil
}

func (r *RemoteClient) RemoteAddr() net.Addr {
	return nil
}
