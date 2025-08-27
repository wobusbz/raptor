package networkentity

type NetworkEntity interface {
	Push(sessionId int64, route string, data []byte) error
	RPC(sessionId int64, route string, data []byte) error
	Close() error
}
