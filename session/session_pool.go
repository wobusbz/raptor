package session

import (
	"game/networkentity"
	"sync"
)

type SessionPool interface {
	NewSession(entity networkentity.NetworkEntity, id int64, UID ...int64) Session
	GetSessionCount() int64
	GetSessionByUID(uid int64) (Session, bool)
	GetSessionByID(id int64) (Session, bool)
	DelSessionByID(id int64)
}

type sessionPoolImpl struct {
	sessionManager   map[int64]*sessionImpl
	uidToSession     map[int64]*sessionImpl
	sessionManagerRw sync.RWMutex
}

func NewSessionPool() SessionPool {
	return &sessionPoolImpl{
		sessionManager: map[int64]*sessionImpl{},
		uidToSession:   map[int64]*sessionImpl{},
	}
}

func (pool *sessionPoolImpl) NewSession(entity networkentity.NetworkEntity, id int64, UID ...int64) Session {
	pool.sessionManagerRw.Lock()
	defer pool.sessionManagerRw.Unlock()
	if id == 0 {
		id = _defaultConnectionSession.SessionID()
	}
	session := NewSession(entity, id)
	session.pool = pool
	pool.sessionManager[session.id] = session

	if len(UID) > 0 && UID[0] != 0 {
		pool.uidToSession[UID[0]] = session
	}

	return session
}

func (pool *sessionPoolImpl) GetSessionCount() int64 {
	return int64(len(pool.sessionManager))
}

func (pool *sessionPoolImpl) GetSessionByUID(uid int64) (Session, bool) {
	pool.sessionManagerRw.RLock()
	defer pool.sessionManagerRw.RUnlock()

	session, ok := pool.uidToSession[uid]
	return session, ok
}

func (pool *sessionPoolImpl) GetSessionByID(id int64) (Session, bool) {
	pool.sessionManagerRw.RLock()
	defer pool.sessionManagerRw.RUnlock()
	session, ok := pool.sessionManager[id]
	return session, ok
}

func (pool *sessionPoolImpl) DelSessionByID(id int64) {
	pool.sessionManagerRw.Lock()
	defer pool.sessionManagerRw.Unlock()

	if session, ok := pool.sessionManager[id]; ok {
		delete(pool.sessionManager, id)
		if _, ok = pool.uidToSession[session.UID()]; ok {
			delete(pool.uidToSession, session.UID())
		}
	}
}

func (pool *sessionPoolImpl) DelSessionByUID(uid int64) {
	pool.sessionManagerRw.Lock()
	defer pool.sessionManagerRw.Unlock()

	if session, ok := pool.uidToSession[uid]; ok {
		delete(pool.uidToSession, uid)
		delete(pool.sessionManager, session.id)
	}
}
