package cluster

import (
	"context"
	"fmt"
	"game/internal/protos"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultPoolSize = 10

type remoteEntry struct {
	clients []protos.RemoteServerClient
	conns   []*grpc.ClientConn
	index   uint32
	size    int
}

func newRemoteEntry(size int) *remoteEntry {
	if size <= 0 {
		size = defaultPoolSize
	}
	return &remoteEntry{
		clients: make([]protos.RemoteServerClient, size),
		conns:   make([]*grpc.ClientConn, size),
		size:    size,
	}
}

func (e *remoteEntry) createdEntry(addr string) error {
	for i := 0; i < e.size; i++ {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return err
		}
		e.conns[i] = conn
		e.clients[i] = protos.NewRemoteServerClient(conn)
	}
	return nil
}

func (e *remoteEntry) pick() protos.RemoteServerClient {
	if len(e.clients) == 0 {
		return nil
	}
	idx := atomic.AddUint32(&e.index, 1)
	return e.clients[int(idx)%len(e.clients)]
}

func (e *remoteEntry) close() {
	for i, rpcconn := range e.conns {
		rpcconn.Close()
		e.conns[i] = nil
		e.clients[i] = nil
	}
}

type grpcRemoteClient struct {
	remotePools    map[string]*remoteEntry
	remoteClientRw sync.RWMutex
	poolSize       int
}

func newGRPCRemoteClient() *grpcRemoteClient {
	return &grpcRemoteClient{
		remotePools: map[string]*remoteEntry{},
		poolSize:    defaultPoolSize,
	}
}

func (r *grpcRemoteClient) SetPoolSize(n int) {
	if n <= 0 {
		return
	}
	r.remoteClientRw.Lock()
	r.poolSize = n
	r.remoteClientRw.Unlock()
}

func (r *grpcRemoteClient) newGRPCRemoteClient(instanceId, addr string) error {
	r.remoteClientRw.Lock()
	defer r.remoteClientRw.Unlock()
	if _, ok := r.remotePools[instanceId]; ok {
		return nil
	}
	entry := newRemoteEntry(r.poolSize)
	if err := entry.createdEntry(addr); err != nil {
		return fmt.Errorf("[NewGRPCRemoteClient] createdEntry %v", err)
	}
	r.remotePools[instanceId] = entry
	return nil
}

func (r *grpcRemoteClient) delGRPCRemoteClient(instanceId string) {
	r.remoteClientRw.Lock()
	if entry, ok := r.remotePools[instanceId]; ok {
		entry.close()
		delete(r.remotePools, instanceId)
	}
	r.remoteClientRw.Unlock()
}

func (r *grpcRemoteClient) call(instanceid string, sessionId int64, data []byte) ([]byte, error) {
	r.remoteClientRw.RLock()
	entry, ok := r.remotePools[instanceid]
	r.remoteClientRw.RUnlock()
	if !ok {
		return nil, fmt.Errorf("[Call] Server %s Not Found", instanceid)
	}
	remote := entry.pick()
	if remote == nil {
		return nil, fmt.Errorf("[Call] Server %s pool empty", instanceid)
	}
	rsp, err := remote.Call(context.TODO(), &protos.CallRequest{
		InstanceId: instanceid,
		SessionId:  sessionId,
		Data:       data,
	})
	if err != nil {
		return nil, err
	}
	return rsp.GetData(), nil
}

func (r *grpcRemoteClient) notify(instanceid string, sessionId int64, data []byte) error {
	r.remoteClientRw.RLock()
	entry, ok := r.remotePools[instanceid]
	r.remoteClientRw.RUnlock()
	if !ok {
		return fmt.Errorf("[Notify] Server %s Not Found", instanceid)
	}
	remote := entry.pick()
	if remote == nil {
		return fmt.Errorf("[Notify] Server %s pool empty", instanceid)
	}
	_, err := remote.Notify(context.TODO(), &protos.NotifyRequest{
		InstanceId: instanceid,
		SessionId:  sessionId,
		Data:       data,
	})
	return err
}

func (r *grpcRemoteClient) Shutdown() {
	r.remoteClientRw.Lock()
	for id, entry := range r.remotePools {
		entry.close()
		delete(r.remotePools, id)
	}
	r.remoteClientRw.Unlock()
}
