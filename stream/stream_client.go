package stream

import (
	"context"
	"errors"
	"fmt"
	"game/internal/protos"
	"log"
	"runtime/debug"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type (
	StreamClient interface {
		Send(m *protos.RemoteMessage) error
		Close() error
	}

	streamClient struct {
		addr        string
		conn        *grpc.ClientConn
		stream      grpc.BidiStreamingClient[protos.RemoteMessage, protos.RemoteMessage]
		chSend      chan pendingMessage
		chDie       chan struct{}
		ctx         context.Context
		cancel      context.CancelFunc
		connState   atomic.Bool
		chReconnect chan struct{}
	}

	pendingMessage struct {
		payload *protos.RemoteMessage
		reply   chan protos.RemoteMessage
	}
)

func NewStreamClient(addr string) (*streamClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &streamClient{
		chSend:      make(chan pendingMessage, 1<<8),
		chDie:       make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
		chReconnect: make(chan struct{}),
	}

	c.conn = conn
	if err = c.newRemoteClient(); err != nil {
		return nil, err
	}

	go c.write()

	return c, nil
}

func (c *streamClient) reconnectSignal() {
	if c.connState.Load() {
		return
	}
	c.connState.Store(true)
	c.chReconnect <- struct{}{}
}

func (c *streamClient) newRemoteClient() error {
	if c.connState.Load() {
		return nil
	}
	stream, err := protos.NewRemoteServerClient(c.conn).Receive(c.ctx)
	if err != nil {
		c.reconnectSignal()
		return err
	}
	c.stream = stream
	return nil
}

func (c *streamClient) Send(m *protos.RemoteMessage) error {
	if c.connState.Load() {
		return fmt.Errorf("[StreamClient/Send] state %v", c.connState.Load())
	}
	select {
	case c.chSend <- pendingMessage{payload: m}:
	default:
		return fmt.Errorf("[StreamClient/Send] queue full")
	}
	return nil
}

func (c *streamClient) Close() error {
	if !c.connState.CompareAndSwap(false, true) {
		return nil
	}
	if c.cancel != nil {
		c.cancel()
	}
	close(c.chDie)
	return errors.Join(c.stream.CloseSend(), c.conn.Close())
}

func (c *streamClient) write() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[StreamClient/Loop] %s\n%v\n", debug.Stack(), r)
		}
		close(c.chSend)
		close(c.chReconnect)
	}()
	c.read()
	for {
		select {
		case <-c.chReconnect:
			err := c.newRemoteClient()
			if err != nil {
				log.Printf("[StreamClient/Loop] reconnect %s\n", err.Error())
				c.connState.Store(false)
				continue
			}

		case data := <-c.chSend:
			if c.connState.Load() {
				continue
			}
			if err := c.stream.Send(data.payload); err != nil {
				c.reconnectSignal()
				log.Printf("[StreamClient/Loop] send %s\n", err.Error())
			}

		case <-c.chDie:
			return
		}
	}
}

func (c *streamClient) read() {
	go func() {
		for {
			_, err := c.stream.Recv()
			if err != nil {
				c.stream.CloseSend()
				return
			}
		}
	}()
}
