package networkentity

import "google.golang.org/protobuf/proto"

type NetworkEntity interface {
	Push(proto.Message) error
	RPC(proto.Message) error
}
