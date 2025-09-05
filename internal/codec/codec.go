package codec

import (
	"bytes"
	"errors"
	"game/internal/packet"
)

const (
	HeadLength    = 4
	MaxPacketSize = 10 << 20
)

var ErrPacketSizeExcced = errors.New("codec: packet size exceed")

type Decoder struct {
	buf  *bytes.Buffer
	size int
	typ  packet.Type
}

func NewDecoder() *Decoder {
	return &Decoder{buf: bytes.NewBuffer(nil), size: -1}
}

func (c *Decoder) forward() error {
	header := c.buf.Next(HeadLength)
	c.typ = packet.Type(header[0])
	if c.typ < packet.Data || c.typ > packet.Forward {
		return packet.ErrWrongPacketType
	}
	c.size = bytesToInt(header[1:])
	if c.size > MaxPacketSize {
		return ErrPacketSizeExcced
	}
	return nil
}

func (d *Decoder) Decode(data []byte) ([]*packet.Packet, error) {
	_, err := d.buf.Write(data)
	if err != nil {
		return nil, err
	}

	if d.buf.Len() < HeadLength {
		return nil, nil
	}

	if d.size < 0 {
		if err = d.forward(); err != nil {
			return nil, err
		}
	}

	var packets []*packet.Packet
	for d.size <= d.buf.Len() {
		packets = append(packets, &packet.Packet{Type: d.typ, Length: d.size, Data: d.buf.Next(d.size)})
		if d.buf.Len() < HeadLength {
			d.size = -1
			break
		}
		if err = d.forward(); err != nil {
			return packets, err
		}
	}

	return packets, nil
}

func Encode(typ packet.Type, data []byte) ([]byte, error) {
	if typ < packet.Data || typ > packet.Forward {
		return nil, packet.ErrWrongPacketType
	}

	p := &packet.Packet{Type: typ, Length: len(data)}
	buf := make([]byte, p.Length+HeadLength)
	buf[0] = byte(p.Type)

	copy(buf[1:HeadLength], intToBytes(p.Length))
	copy(buf[HeadLength:], data)

	return buf, nil
}

func bytesToInt(b []byte) int {
	return int(b[2]) | int(b[1])<<8 | int(b[0])<<16
}

func intToBytes(n int) []byte {
	var b [3]byte
	b[0] = byte(n >> 16)
	b[1] = byte(n >> 8)
	b[2] = byte(n)
	return b[:]
}
