package message

import "maps"

type Type byte

const (
	Request  Type = 0x00
	Notify   Type = 0x01
	Response Type = 0x02
	Push     Type = 0x03
)

type Message struct {
	Type  Type
	Route string
	ID    uint
	Data  []byte
}

var dict = map[int]string{1: "CENT/User/C2SLogin"}

func Decode(data []byte) *Message {
	var m = &Message{}
	var offset int
	for offset = 0; offset < len(data); offset++ {
		b := data[offset]
		m.ID |= uint(b&0x7F) << (uint(offset) * 7)
		if b&0x80 == 0 {
			break
		}
	}
	if offset < len(data) {
		m.Data = data[offset+1:]
	} else {
		m.Data = nil
	}
	m.Route = dict[int(m.ID)]
	return m
}

func Encode(m *Message) []byte {
	id := uint(m.ID)
	var header []byte
	for {
		b := byte(id & 0x7F)
		id >>= 7
		if id != 0 {
			header = append(header, b|0x80)
		} else {
			header = append(header, b)
			break
		}
	}
	return append(header, m.Data...)
}

func SetRouteDict(newdict map[int]string) {
	maps.Copy(dict, newdict)
}
