package agent

import (
	"encoding/json"
	"fmt"
	"game/internal/codec"
	"game/internal/message"
	"game/internal/packet"
	"game/session"
	"log"
	"net"
	"runtime/debug"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
)

func hrd(now int64) []byte {
	hrdata := map[string]any{"code": 200, "sys": map[string]any{"heartbeat": time.Second, "servertime": now}}
	data, _ := json.Marshal(hrdata)
	hrd, _ := codec.Encode(packet.Heartbeat, data)
	return hrd
}

type (
	rpcHandler func(session session.Session, msg *message.Message) error

	Agent struct {
		*codec.Decoder
		conn             net.Conn
		session          session.Session
		sessionPool      session.SessionPool
		chDie            chan struct{}
		lastAt           atomic.Int64
		heartbeatTimeout time.Duration
		rpcHandler       rpcHandler
		sendch           chan pendingMessage
		state            atomic.Bool
	}

	pendingMessage struct {
		typ     message.Type
		id      int
		payload []byte
	}
)

func NewAgent(conn net.Conn, sessionPool session.SessionPool, rpcHandler rpcHandler) *Agent {
	a := &Agent{
		Decoder:          codec.NewDecoder(),
		conn:             conn,
		sessionPool:      sessionPool,
		chDie:            make(chan struct{}),
		rpcHandler:       rpcHandler,
		heartbeatTimeout: time.Second * 5,
		sendch:           make(chan pendingMessage, 1<<8),
	}

	a.lastAt.Store(time.Now().Unix())
	go a.write()
	a.session = a.sessionPool.NewSession(nil, 0)
	return a
}

func (a *Agent) Session() session.Session {
	return a.session
}

func (a *Agent) Push(pb proto.Message) error {
	if a.state.Load() {
		return fmt.Errorf("[Agent/Push] %v Agent closed", a.session.ID())
	}
	pber, ok := pb.(interface{ ID() uint32 })
	if !ok {
		return fmt.Errorf("[Agent/Push] Reflection GetMsgId failure")
	}
	pbData, err := proto.Marshal(pb)
	if err != nil {
		return fmt.Errorf("[Agent/Push] protobuf Marshal failed %v", err)
	}
	return a.Response(int(pber.ID()), pbData)
}

func (a *Agent) Response(id int, pbData []byte) error {
	if a.state.Load() {
		return fmt.Errorf("[Agent/Response] %v Agent closed", a.session.ID())
	}
	return a.send(pendingMessage{typ: message.Response, id: id, payload: pbData})
}

func (a *Agent) RPC(pb proto.Message) error {
	if a.state.Load() {
		return fmt.Errorf("[Agent/RPC] %v Agent closed", a.session.ID())
	}
	pber, ok := pb.(interface{ Route() string })
	if !ok {
		return fmt.Errorf("[Agent/RPC] Reflection GetSvrName failure")
	}
	pbdata, err := proto.Marshal(pb)
	if err != nil {
		return fmt.Errorf("[Agent/RPC] %w", err)
	}
	m := message.Message{
		Type:  message.Notify,
		Route: pber.Route(),
		Data:  pbdata,
	}
	return a.rpcHandler(a.session, &m)
}

func (a *Agent) UpdateHeartbeat() {
	a.lastAt.Store(time.Now().Unix())
}

func (a *Agent) Close() error {
	if !a.state.CompareAndSwap(false, true) {
		return nil
	}
	close(a.chDie)
	return a.conn.Close()
}

func (a *Agent) send(m pendingMessage) error {
	select {
	case a.sendch <- m:
	default:
		return fmt.Errorf("[Agent/Send] queue full")
	}
	return nil
}

func (a *Agent) write() {
	ticker := time.NewTicker(a.heartbeatTimeout)
	writeCh := make(chan []byte, 1<<8)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("%s\n%v\n", string(debug.Stack()), r)
		}
		ticker.Stop()
		a.Close()
		close(writeCh)
	}()
	for {
		select {
		case vtime := <-ticker.C:
			if vtime.Unix()-int64(a.heartbeatTimeout.Seconds()*2) > a.lastAt.Load() {
				log.Printf("[Agent/Write] Session heartbeat timeout, LastTime=%d, Deadline=%d\n", a.lastAt.Load(), vtime.Unix()-a.lastAt.Load())
				return
			}
			hrd(vtime.UTC().Unix())

		case data := <-writeCh:
			if _, err := a.conn.Write(data); err != nil {
				log.Printf("[Agent/Write] SessionId %d net write %s\n", a.session.ID(), err.Error())
				return
			}

		case data := <-a.sendch:
			pb, err := codec.Encode(packet.Data, message.Encode(&message.Message{Type: data.typ, ID: uint(data.id), Data: data.payload}))
			if err != nil {
				log.Printf("[Agent/Write] codec.Encode %s\n", err.Error())
				continue
			}
			writeCh <- pb

		case <-a.chDie:
			return
		}
	}
}
