package codec

import (
	"bytes"
	"errors"
)

const (
	HeadLength    = 4
	MaxPacketSize = 8 << 20
)

var ErrPacketSizeExcced = errors.New("codec: packet size exceed")

type Decoder struct {
	buf  *bytes.Buffer
	size int
	typ  Type
}

func NewDecoder() *Decoder {
	return &Decoder{buf: bytes.NewBuffer(nil), size: -1}
}

func (c *Decoder) forward() error {
	header := c.buf.Next(HeadLength)
	c.typ = Type(header[0])
	if c.typ < Data || c.typ > Forward {
		return ErrWrongPacketType
	}
	c.size = bytesToInt(header[1:])
	if c.size > MaxPacketSize {
		return ErrPacketSizeExcced
	}
	return nil
}

func (d *Decoder) Decode(data []byte) ([]*Packet, error) {
	_, err := d.buf.Write(data)
	if err != nil {
		return nil, err
	}

	if d.buf.Len() < HeadLength {
		return nil, err
	}

	if d.size < 0 {
		if err = d.forward(); err != nil {
			return nil, err
		}
	}

	var packets []*Packet
	for d.size <= d.buf.Len() {
		packets = append(packets, &Packet{Type: d.typ, Length: d.size, Data: d.buf.Next(d.size)})
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

func Encode(typ Type, data []byte) ([]byte, error) {
	if typ < Data || typ > Forward {
		return nil, ErrWrongPacketType
	}

	p := &Packet{Type: typ, Length: len(data)}
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
