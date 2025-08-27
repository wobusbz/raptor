package cluster

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"game/internal/protos"
	"game/networkentity"
	"game/session"
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
	instances           *ServiceInstance
	rpcClient           *rpcClient
	members             map[string][]*ServiceInstance
	membersrw           sync.RWMutex
	stopc               chan struct{}
	closeonce           sync.Once
	heartbeatInterval   time.Duration
	heartbeatTimeout    time.Duration
	RemoteClients       map[string][]*RemoteClient
	RemoteClientRw      sync.RWMutex
}

func NewGRPCDiscoveryClient(rpcClient *rpcClient) *grpcDiscoveryClient {
	return &grpcDiscoveryClient{
		rpcClient:         rpcClient,
		members:           map[string][]*ServiceInstance{},
		stopc:             make(chan struct{}),
		heartbeatInterval: 3 * time.Second,
		heartbeatTimeout:  10 * time.Second,
		RemoteClients:     map[string][]*RemoteClient{},
	}
}

func (c *grpcDiscoveryClient) setGRPCDiscoveryServer(grpcDiscoveryServer *grpcDiscoveryServer) {
	c.grpcDiscoveryServer = grpcDiscoveryServer
}

func (c *grpcDiscoveryClient) getCurServerInstance() *protos.ServiceInstance {
	return c.instances.Instances
}

func (c *grpcDiscoveryClient) register(frontend, master bool, name, serverAddr, targetAddr string) error {
	b := make([]byte, 16)
	_, _ = rand.Read(b)

	c.instances = &ServiceInstance{
		Instances: &protos.ServiceInstance{ServiceName: name, InstanceId: hex.EncodeToString(b), Addr: serverAddr, Frontend: frontend},
		master:    master,
	}
	if master {
		return nil
	}
	rpcconn, err := c.rpcClient.getConnPool(targetAddr)
	if err != nil {
		return err
	}
	pbrsvr := protos.NewRegistryServerClient(rpcconn.Get())
	_, err = pbrsvr.Register(context.TODO(), &protos.RegisterRequest{Instances: c.instances.Instances})
	if err != nil {
		return err
	}
	go c.heartbeat(targetAddr)
	return nil
}

func (c *grpcDiscoveryClient) NewMembers(ctx context.Context, in *protos.NewMembersRequest) (*protos.NewMembersResponse, error) {
	c.newMembers(false, in.GetInstances())
	return &protos.NewMembersResponse{}, nil
}

func (c *grpcDiscoveryClient) DelMembers(ctx context.Context, in *protos.DelMembersRequest) (*protos.DelMembersResponse, error) {
	c.delMembers(in.GetServiceName(), in.GetInstanceId())
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
				Instances: c.instances.Instances,
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

	c.RemoteClientRw.Lock()
	c.RemoteClients[instance.GetServiceName()] = append(c.RemoteClients[instance.GetServiceName()], newRemoteClient(instance.GetAddr()))
	c.RemoteClientRw.Unlock()
}

func (c *grpcDiscoveryClient) delMembers(serverName, instanceId string) {
	for _, s := range c.getMembers(serverName) {
		c.RemoteClientRw.Lock()
		remoteClients, ok := c.RemoteClients[serverName]
		if ok {
			c.RemoteClients[serverName] = slices.DeleteFunc(remoteClients, func(c *RemoteClient) bool {
				return c.addr == s.Instances.Addr
			})
		}
		c.RemoteClientRw.Unlock()
	}
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

func (c *grpcDiscoveryClient) updateMembers(instance *protos.ServiceInstance) error {
	c.membersrw.Lock()
	defer c.membersrw.Unlock()
	servers, ok := c.members[instance.GetServiceName()]
	if !ok {
		c.members[instance.GetServiceName()] = append(c.members[instance.GetServiceName()], &ServiceInstance{
			Instances: instance,
			expireAt:  time.Now().Add(c.heartbeatTimeout),
		})
	} else {
		for _, s := range servers {
			if s.Instances.GetInstanceId() != instance.GetInstanceId() {
				continue
			}
			s.expireAt = time.Now().Add(c.heartbeatTimeout)
		}
	}
	return nil
}

func (c *grpcDiscoveryClient) SelectRemoteClient() (map[string]networkentity.NetworkEntity, bool) {
	c.RemoteClientRw.RLock()
	defer c.RemoteClientRw.RUnlock()
	var entitys = map[string]networkentity.NetworkEntity{}
	for k, rcs := range c.RemoteClients {
		entitys[k] = rcs[0]
	}
	return entitys, true
}

func (c *grpcDiscoveryClient) notifyOnsession(session session.Session, remoteClients map[string]networkentity.NetworkEntity) {
	onSessionConnectMessage := &protos.OnSessionConnectMessage{ID: session.ID(), Instances: map[string]*protos.ServiceInstance{}}
	onSessionConnectMessage.Instances[c.instances.Instances.GetServiceName()] = c.instances.Instances
	for kname, rs := range remoteClients {
		rsClient, ok := rs.(*RemoteClient)
		if !ok {
			continue
		}
		for _, v := range c.getMembers(kname) {
			if rsClient.addr != v.Instances.Addr {
				continue
			}
			onSessionConnectMessage.Instances[v.Instances.GetServiceName()] = v.Instances
			break
		}
	}
	for _, rs := range remoteClients {
		rsClient, ok := rs.(*RemoteClient)
		if !ok {
			continue
		}
		rsClient.stream.Send(
			&protos.RemoteMessage{Kind: protos.RemoteMessage_KIND_ON_SESSION_CONNECT, OnSessionConnectMessage: onSessionConnectMessage},
		)
	}
}

func (c *grpcDiscoveryClient) GetRemoteClient(name, addr string) (networkentity.NetworkEntity, bool) {
	c.RemoteClientRw.RLock()
	defer c.RemoteClientRw.RUnlock()
	remoteClients, ok := c.RemoteClients[name]
	if !ok {
		return nil, false
	}
	for _, v := range remoteClients {
		if v.addr != addr {
			continue
		}
		return v, true
	}
	return nil, false
}

func (c *grpcDiscoveryClient) Shutdown() {
	c.closeonce.Do(func() {
		close(c.stopc)
		c.RemoteClientRw.RLock()
		for _, rc := range c.RemoteClients {
			for _, rccs := range rc {
				rccs.Close()
			}
		}
		c.RemoteClientRw.RUnlock()
	})
}
