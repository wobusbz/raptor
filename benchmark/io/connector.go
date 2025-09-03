package io

import (
	"game/internal/codec"
	"game/internal/message"
	"game/internal/packet"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"
)

type (
	Callback func(data any)

	Connector struct {
		conn   net.Conn
		codec  *codec.Decoder
		die    chan struct{}
		chSend chan []byte

		muEvents sync.RWMutex
		events   map[string]Callback

		muResponses sync.RWMutex
		responses   map[uint64]Callback

		connectedCallback func(data []byte)

		state atomic.Bool
	}
)

func NewConnector() *Connector {
	return &Connector{
		die:       make(chan struct{}),
		codec:     codec.NewDecoder(),
		chSend:    make(chan []byte, 64),
		events:    map[string]Callback{},
		responses: map[uint64]Callback{},
	}
}

func (c *Connector) Start(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}

	c.conn = conn

	go c.write()

	go c.read()

	return nil
}

func (c *Connector) OnConnected(callback func([]byte)) {
	c.connectedCallback = callback
}

func (c *Connector) Request(route string, v proto.Message, callback Callback) error {
	data, err := serialize(v)
	if err != nil {
		return err
	}
	msg := &message.Message{
		Type: message.Request,
		ID:   1,
		Data: data,
	}
	if err = c.sendMessage(msg); err != nil {
		return err
	}
	return nil
}

func (c *Connector) On(event string, callback Callback) {
	c.muEvents.Lock()
	defer c.muEvents.Unlock()

	c.events[event] = callback
}

func (c *Connector) Close() {
	c.conn.Close()
	close(c.die)
}

func (c *Connector) eventHandler(event string) (Callback, bool) {
	c.muEvents.RLock()
	defer c.muEvents.RUnlock()

	cb, ok := c.events[event]
	return cb, ok
}

func (c *Connector) sendMessage(msg *message.Message) error {
	data, err := codec.Encode(packet.Data, message.Encode(msg))
	if err != nil {
		return err
	}
	c.send(data)
	return nil
}

func (c *Connector) write() {
	defer func() {
		close(c.chSend)
		c.state.Store(true)
	}()

	for {
		select {
		case data := <-c.chSend:
			if _, err := c.conn.Write(data); err != nil {
				log.Println(err.Error())
				c.Close()
				return
			}

		case <-c.die:
			return
		}
	}
}

func (c *Connector) send(data []byte) {
	if c.state.Load() {
		return
	}
	c.chSend <- data
}

func (c *Connector) read() {
	buf := make([]byte, 2048)

	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			log.Println(err.Error())
			c.Close()
			return
		}
		// log.Println(string(buf[:n]))
		packets, err := c.codec.Decode(buf[:n])
		if err != nil {
			// log.Println(err.Error())
			// c.Close()
			// return
			continue
		}

		for i := range packets {
			p := packets[i]
			c.processPacket(p)
		}
	}
}

func (c *Connector) processPacket(p *packet.Packet) {
	switch p.Type {
	case packet.Heartbeat:
		c.connectedCallback(p.Data)
	case packet.Data:
		c.processMessage(message.Decode(p.Data))

	case packet.Kick:
		c.Close()
	}
}

func (c *Connector) processMessage(msg *message.Message) {
	switch msg.Type {
	case message.Push:
		cb, ok := c.eventHandler(msg.Route)
		if !ok {
			log.Println("event handler not found", msg.Route)
			return
		}

		cb(msg.Data)

	case message.Response:
		// cb, ok := c.responseHandler(msg.ID)
		// if !ok {
		// 	log.Println("response handler not found", msg.ID)
		// 	return
		// }
		//
		// cb(msg.Data)
		// c.setResponseHandler(msg.ID, nil)
	}
}

func serialize(v proto.Message) ([]byte, error) {
	data, err := proto.Marshal(v)
	if err != nil {
		return nil, err
	}
	return data, nil
}
