package session

import (
	"game/networkentity"
	"sync"
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
	RPC(message proto.Message) error
	Response(message proto.Message) error
	FindRoutes(name string) (networkentity.NetworkEntity, bool)
	DelRoutes(name string)
	BindServer(server string, entity networkentity.NetworkEntity)
}

type SessionPool interface {
	NewSession(entity networkentity.NetworkEntity, frontend bool, UID ...string) Session
	OnConnectionNewSession(entity networkentity.NetworkEntity, id int64, UID ...string) Session
	GetSessionCount() int64
	GetSessionByUID(uid string) (Session, bool)
	GetSessionByID(id int64) (Session, bool)
	DelSessionByID(id int64)
}

type sessionImpl struct {
	id           int64
	uid          atomic.Int64
	entity       networkentity.NetworkEntity
	lastTime     int64
	metaProvider MetadataProvider
	Router       *Router
	pool         *sessionPoolImpl
}

func NewSession(entity networkentity.NetworkEntity, id int64) *sessionImpl {
	return &sessionImpl{
		id:           id,
		metaProvider: DirectMetadataProvider{},
		entity:       entity,
		Router:       NewRouter(),
	}
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
	pbdata, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	return s.entity.Push(s.id, "CENT/User/C2SLogin", pbdata)
}

func (s *sessionImpl) RPC(message proto.Message) error {
	metaProvider, err := s.metaProvider.Extract(message)
	if err != nil {
		// return err
	}
	pbdata, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	_ = metaProvider
	return s.entity.RPC(s.id, "CENT/User/C2SLogin", pbdata)
}

func (s *sessionImpl) Response(message proto.Message) error {
	return s.entity.Response("", "", nil)
}

func (s *sessionImpl) FindRoutes(name string) (networkentity.NetworkEntity, bool) {
	return s.Router.FindServer(name)
}

func (s *sessionImpl) DelRoutes(name string) {
	s.Router.DeleteServer(name)
}

func (s *sessionImpl) BindServer(server string, entity networkentity.NetworkEntity) {
	s.Router.BindServer(server, entity)
}

type sessionPoolImpl struct {
	sessionManager   map[int64]*sessionImpl
	sessionManagerRw sync.RWMutex
}

func NewSessionPool() SessionPool {
	return &sessionPoolImpl{sessionManager: map[int64]*sessionImpl{}}
}

func (pool *sessionPoolImpl) NewSession(entity networkentity.NetworkEntity, frontend bool, UID ...string) Session {
	pool.sessionManagerRw.Lock()
	session := NewSession(entity, _defaultConnectionSession.SessionID())
	pool.sessionManager[session.id] = session
	pool.sessionManagerRw.Unlock()
	return session
}

func (pool *sessionPoolImpl) OnConnectionNewSession(entity networkentity.NetworkEntity, id int64, UID ...string) Session {
	pool.sessionManagerRw.Lock()
	session := NewSession(entity, id)
	pool.sessionManager[session.id] = session
	pool.sessionManagerRw.Unlock()
	return session
}

func (pool *sessionPoolImpl) GetSessionCount() int64 {
	return 0
}

func (pool *sessionPoolImpl) GetSessionByUID(uid string) (Session, bool) {
	return NewSession(nil, 0), true
}

func (pool *sessionPoolImpl) GetSessionByID(id int64) (Session, bool) {
	pool.sessionManagerRw.RLock()
	defer pool.sessionManagerRw.RUnlock()
	session, ok := pool.sessionManager[id]
	return session, ok
}

func (pool *sessionPoolImpl) DelSessionByID(id int64) {
	pool.sessionManagerRw.Lock()
	delete(pool.sessionManager, id)
	pool.sessionManagerRw.Unlock()
}
