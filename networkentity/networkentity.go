package networkentity

import (
	"net"

	"google.golang.org/protobuf/proto"
)

type NetworkEntity interface {
	Push(sessionId int64, route string, data []byte) error
	RPC(sessionId int64, route string, data []byte) error
	Response(modelName, method string, message proto.Message) error
	Close() error
	RemoteAddr() net.Addr
}
