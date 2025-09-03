package session

type (
	LifetimeHandler func(Session)

	lifetime struct {
		onClosed []LifetimeHandler
	}
)

var Lifetime = &lifetime{}

func (lt *lifetime) OnClosed(h LifetimeHandler) {
	lt.onClosed = append(lt.onClosed, h)
}

func (lt *lifetime) Close(s Session) {
	if len(lt.onClosed) < 1 {
		return
	}

	for _, h := range lt.onClosed {
		h(s)
	}
}
