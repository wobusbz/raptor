package session

import (
	"game/networkentity"
	"sync/atomic"

	"google.golang.org/protobuf/proto"
)

type defaultConnectionSession struct {
	id    atomic.Int64
	count atomic.Int64
}

func (d *defaultConnectionSession) Increment() {
	d.count.Add(1)
}

func (d *defaultConnectionSession) Decrement() {
	d.count.Add(-1)
}

func (d *defaultConnectionSession) Count() int64 {
	return d.count.Load()
}

func (d *defaultConnectionSession) Reset() {
	d.count.Store(0)
}

func (d *defaultConnectionSession) SessionID() int64 {
	return d.id.Add(1)
}

var _defaultConnectionSession = &defaultConnectionSession{}

type Session interface {
	ID() int64
	UID() int64
	Push(message proto.Message) error
	Response(id int, data []byte) error
	RPC(message proto.Message) error
	Routers() *Router
}

type sessionImpl struct {
	id       int64
	uid      atomic.Int64
	entity   networkentity.NetworkEntity
	lastTime int64
	Router   *Router
	pool     *sessionPoolImpl
}

func NewSession(entity networkentity.NetworkEntity, id int64) *sessionImpl {
	return &sessionImpl{id: id, entity: entity, Router: NewRouter()}
}

func (s *sessionImpl) ID() int64 {
	return s.id
}

func (s *sessionImpl) UID() int64 {
	return s.uid.Load()
}

func (s *sessionImpl) Bind(uid int64) {
	s.uid.Store(uid)
}

func (s *sessionImpl) Push(message proto.Message) error {
	return s.entity.Push(message)
}

func (s *sessionImpl) Response(id int, data []byte) error {
	return nil
}

func (s *sessionImpl) RPC(message proto.Message) error {
	return s.entity.RPC(message)
}

func (s *sessionImpl) Broadcast(message proto.Message) {

}

func (s *sessionImpl) Routers() *Router {
	return s.Router
}
