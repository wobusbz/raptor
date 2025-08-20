package cluster

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"game/internal/protos"
	"log"
	"slices"
	"sync"
	"time"
)

type ServiceInstance struct {
	Instances *protos.ServiceInstance
	expireAt  time.Time
	master    bool
}

type grpcDiscoveryClient struct {
	protos.UnimplementedMembersServerServer
	grpcDiscoveryServer *grpcDiscoveryServer
	instances           *protos.ServiceInstance
	rpcClient           *rpcClient
	members             map[string][]*ServiceInstance
	membersrw           sync.RWMutex
	stopc               chan struct{}
	closeonce           sync.Once
	grpcRemoteClient    *grpcRemoteClient
	heartbeatInterval   time.Duration
	heartbeatTimeout    time.Duration
}

func NewGRPCDiscoveryClient(rpcClient *rpcClient, grpcRemoteClient *grpcRemoteClient) *grpcDiscoveryClient {
	return &grpcDiscoveryClient{
		rpcClient:         rpcClient,
		members:           map[string][]*ServiceInstance{},
		stopc:             make(chan struct{}),
		grpcRemoteClient:  grpcRemoteClient,
		heartbeatInterval: 3 * time.Second,
		heartbeatTimeout:  10 * time.Second,
	}
}

func (c *grpcDiscoveryClient) setGRPCDiscoveryServer(grpcDiscoveryServer *grpcDiscoveryServer) {
	c.grpcDiscoveryServer = grpcDiscoveryServer
}

func (c *grpcDiscoveryClient) getCurServerInstance() *protos.ServiceInstance {
	return c.instances
}

func (c *grpcDiscoveryClient) register(master bool, name, serverAddr, targetAddr string) error {
	b := make([]byte, 16)
	_, _ = rand.Read(b)

	c.instances = &protos.ServiceInstance{ServiceName: name, InstanceId: hex.EncodeToString(b), Addr: serverAddr}
	if master {
		return nil
	}
	rpcconn, err := c.rpcClient.getConnPool(targetAddr)
	if err != nil {
		return err
	}
	pbrsvr := protos.NewRegistryServerClient(rpcconn.Get())
	_, err = pbrsvr.Register(context.TODO(), &protos.RegisterRequest{Instances: c.instances})
	if err != nil {
		return err
	}
	go c.heartbeat(targetAddr)
	return nil
}

func (c *grpcDiscoveryClient) NewMembers(ctx context.Context, in *protos.NewMembersRequest) (*protos.NewMembersResponse, error) {
	c.newMembers(false, in.GetInstances())
	return &protos.NewMembersResponse{}, c.grpcRemoteClient.newGRPCRemoteClient(in.Instances.GetInstanceId(), in.Instances.GetAddr())
}

func (c *grpcDiscoveryClient) DelMembers(ctx context.Context, in *protos.DelMembersRequest) (*protos.DelMembersResponse, error) {
	c.delMembers(in.GetServiceName(), in.GetInstanceId())
	c.grpcRemoteClient.delGRPCRemoteClient(in.GetInstanceId())
	return &protos.DelMembersResponse{}, nil
}

func (c *grpcDiscoveryClient) heartbeat(targetAddr string) {
	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rpcconn, err := c.rpcClient.getConnPool(targetAddr)
			if err != nil {
				log.Println("heartbeat getConnPool error:", err)
				continue
			}
			_, err = protos.NewRegistryServerClient(rpcconn.Get()).Heartbeat(context.TODO(), &protos.HeartbeatRequest{
				ServiceName: c.instances.ServiceName,
				InstanceId:  c.instances.InstanceId,
			})
			if err != nil {
				log.Println("heartbeat send error:", err)
			}
		case <-c.stopc:
			return
		}
	}
}

func (c *grpcDiscoveryClient) newMembers(master bool, instance *protos.ServiceInstance) {
	c.membersrw.Lock()

	servers, ok := c.members[instance.GetServiceName()]
	if ok {
		c.members[instance.GetServiceName()] = slices.DeleteFunc(servers, func(s *ServiceInstance) bool {
			return s.Instances.GetInstanceId() == instance.GetInstanceId()
		})
	}
	c.members[instance.GetServiceName()] = append(c.members[instance.GetServiceName()], &ServiceInstance{
		master:    master,
		Instances: instance,
		expireAt:  time.Now().Add(c.heartbeatTimeout),
	})
	c.membersrw.Unlock()
}

func (c *grpcDiscoveryClient) delMembers(serverName, instanceId string) {
	c.membersrw.Lock()
	servers, ok := c.members[serverName]
	if ok {
		c.members[serverName] = slices.DeleteFunc(servers, func(s *ServiceInstance) bool {
			return s.Instances.GetInstanceId() == instanceId
		})
	}
	c.membersrw.Unlock()
}

func (c *grpcDiscoveryClient) getMembers(serverName string) []*ServiceInstance {
	c.membersrw.RLock()
	defer c.membersrw.RUnlock()
	members, ok := c.members[serverName]
	if !ok {
		return nil
	}
	return members
}

func (c *grpcDiscoveryClient) getAllMembers() map[string][]*ServiceInstance {
	c.membersrw.RLock()
	defer c.membersrw.RUnlock()
	out := make(map[string][]*ServiceInstance, len(c.members))
	for k, v := range c.members {
		cp := make([]*ServiceInstance, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

func (c *grpcDiscoveryClient) updateMembers(serverName, instanceId string) error {
	c.membersrw.Lock()
	defer c.membersrw.Unlock()
	servers, ok := c.members[serverName]
	if !ok {
		return fmt.Errorf("Server %s NOT FOUND", serverName)
	}
	for _, s := range servers {
		if s.Instances.GetInstanceId() != instanceId {
			continue
		}
		s.expireAt = time.Now().Add(c.heartbeatTimeout)
	}
	return nil
}

func (c *grpcDiscoveryClient) Shutdown() {
	c.closeonce.Do(func() { close(c.stopc) })
}
