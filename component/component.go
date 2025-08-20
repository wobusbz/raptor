package component

type Component interface {
	Init()
	Shutdown()
	OnSessionDisconnect()
	OnSessionConnect()
}
