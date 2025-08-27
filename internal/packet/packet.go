package packet

import (
	"errors"
	"fmt"
)

type Type byte

const (
	Data      Type = 0x01
	Heartbeat Type = 0x02
	Kick      Type = 0x03
	Forward   Type = 0x04
)

var ErrWrongPacketType = errors.New("wrong packet type")

type Packet struct {
	Type   Type
	Length int
	Data   []byte
}

func New() *Packet {
	return &Packet{}
}

func (p *Packet) String() string {
	return fmt.Sprintf("Type: %d, Length: %d, Data: %s", p.Type, p.Length, string(p.Data))
}
