package message

import (
	"fmt"
	"testing"
)

func TestMessage(t *testing.T) {
	msg := &Message{
		Type: Notify,
		ID:   10000,
		Data: []byte("hello"),
	}
	enc := Encode(msg)
	dec := Decode(enc)
	fmt.Println(dec.Type, dec.ID, string(dec.Data), dec.Route)

}
