package cluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"game/component"
	"game/internal/codec"
	"game/internal/message"
	"game/internal/packet"
	"game/internal/protos"
	"game/networkentity"
	"game/session"
	"log"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

func hrd() []byte {
	hrdata := map[string]any{
		"code": 200,
		"sys":  map[string]any{"heartbeat": time.Second, "servertime": time.Now().UTC().Unix()},
	}
	data, _ := json.Marshal(hrdata)
	hrd, _ := codec.Encode(packet.Heartbeat, data)
	return hrd
}

var _ networkentity.NetworkEntity = (*agent)(nil)

type agent struct {
	conn                net.Conn
	sessionPool         session.SessionPool
	session             session.Session
	heartbeatTimeout    time.Duration
	writeTimeout        time.Duration
	lastAt              int64
	sendch              chan message.Message
	pack                *codec.Decoder
	comps               map[string]component.Component
	grpcDiscoveryClient *grpcDiscoveryClient
	diec                chan struct{}
	closeState          atomic.Bool
}

func newAgent(
	conn net.Conn,
	sessionPoll session.SessionPool,
	comps map[string]component.Component,
	grpcDiscoveryClient *grpcDiscoveryClient,
) *agent {

	a := &agent{
		conn:                conn,
		sessionPool:         sessionPoll,
		sendch:              make(chan message.Message, 1024),
		pack:                codec.NewDecoder(),
		comps:               comps,
		grpcDiscoveryClient: grpcDiscoveryClient,
		heartbeatTimeout:    time.Second * 5,
		lastAt:              time.Now().Unix(),
		diec:                make(chan struct{}, 1),
	}
	entitys, ok := a.grpcDiscoveryClient.SelectRemoteClient()
	if ok {
		a.session = a.sessionPool.NewSession(a, true)
		a.grpcDiscoveryClient.notifyOnsession(a.session, entitys)
		for kname, entity := range entitys {
			a.session.BindServer(kname, entity)
		}
	}
	return a
}

func (a *agent) write() {
	ticker := time.NewTicker(a.heartbeatTimeout)
	chWrite := make(chan []byte, 1024)
	defer func() {
		ticker.Stop()
		close(chWrite)
		close(a.sendch)
		a.conn.Close()
		a.sessionPool.DelSessionByID(a.session.ID())
		a.session.RPC(&protos.RemoteMessage{
			Kind:                          protos.RemoteMessage_KIND_ON_SESSION_DISCONNECT,
			OnSessionDisconnectionMessage: &protos.OnSessionDisconnectionMessage{ID: a.session.ID()},
		})
	}()
	for {
		select {
		case data := <-chWrite:
			if _, err := a.conn.Write(data); err != nil {
				log.Println(err.Error())
				return
			}
			atomic.StoreInt64(&a.lastAt, time.Now().Add(a.heartbeatTimeout).Unix())
		case v := <-ticker.C:
			deadline := time.Now().Add(-2 * a.heartbeatTimeout).Unix()
			if atomic.LoadInt64(&a.lastAt) < deadline {
				log.Printf("Session heartbeat timeout, LastTime=%d, Deadline=%d \n", atomic.LoadInt64(&a.lastAt), deadline)
				return
			}
			if atomic.LoadInt64(&a.lastAt) < int64(a.heartbeatTimeout/time.Second) {
				continue
			}
			chWrite <- hrd()
			atomic.StoreInt64(&a.lastAt, v.Add(a.heartbeatTimeout).Unix())
		case msg := <-a.sendch:
			chWrite <- message.Encode(&msg)
		case <-a.diec:
			return
		}
	}
}

func (a *agent) Close() error {
	if !a.closeState.CompareAndSwap(false, true) {
		return nil
	}
	close(a.diec)
	a.sessionPool.GetSessionByID(a.session.ID())
	for _, s := range a.comps {
		s.OnSessionDisconnect(a.session)
	}
	return nil
}

func (a *agent) Push(sessionId int64, route string, data []byte) error {
	if a.closeState.Load() {
		return errors.New("[Agent/Push] closed")
	}
	select {
	case a.sendch <- message.Message{Route: route, Data: data}:
	default:
		return errors.New("[Agent/Push] sendch full")
	}
	return nil
}

func (a *agent) RPC(sessionId int64, route string, data []byte) error {
	if a.closeState.Load() {
		return errors.New("[Agent/Push] closed")
	}
	routes := strings.Split(route, "/")
	remoteClient, ok := a.session.FindRoutes(routes[0])
	if !ok {
		return fmt.Errorf("[Agent/RPC] Server %s not found", routes[0])
	}
	return remoteClient.RPC(sessionId, route, data)
}
