package cluster

import (
	"errors"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type connPool struct {
	index uint32
	v     []*grpc.ClientConn
}

type rpcClient struct {
	sync.RWMutex
	isClosed bool
	pools    map[string]*connPool
}

func newConnArray(maxSize uint, addr string) (*connPool, error) {
	a := &connPool{
		index: 0,
		v:     make([]*grpc.ClientConn, maxSize),
	}
	if err := a.init(addr); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *connPool) init(addr string) error {
	for i := range a.v {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			a.Close()
			return err
		}
		a.v[i] = conn
	}
	return nil
}

func (a *connPool) Get() *grpc.ClientConn {
	next := atomic.AddUint32(&a.index, 1) % uint32(len(a.v))
	return a.v[next]
}

func (a *connPool) Close() {
	for i, c := range a.v {
		if c != nil {
			err := c.Close()
			if err != nil {
			}
			a.v[i] = nil
		}
	}
}

func newRPCClient() *rpcClient {
	return &rpcClient{
		pools: make(map[string]*connPool),
	}
}

func (c *rpcClient) getConnPool(addr string) (*connPool, error) {
	c.RLock()
	if c.isClosed {
		c.RUnlock()
		return nil, errors.New("rpc client is closed")
	}
	array, ok := c.pools[addr]
	c.RUnlock()
	if !ok {
		var err error
		array, err = c.createConnPool(addr)
		if err != nil {
			return nil, err
		}
	}
	return array, nil
}

func (c *rpcClient) createConnPool(addr string) (*connPool, error) {
	c.Lock()
	defer c.Unlock()
	array, ok := c.pools[addr]
	if !ok {
		var err error
		array, err = newConnArray(10, addr)
		if err != nil {
			return nil, err
		}
		c.pools[addr] = array
	}
	return array, nil
}

func (c *rpcClient) closePool() {
	c.Lock()
	if !c.isClosed {
		c.isClosed = true
		for _, array := range c.pools {
			array.Close()
		}
	}
	c.Unlock()
}
