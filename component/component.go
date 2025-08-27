package component

import "game/session"

type Component interface {
	Init()
	Shutdown()
	OnSessionDisconnect(session.Session)
	OnSessionConnect(session.Session)
}
