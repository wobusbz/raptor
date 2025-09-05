package cluster

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"game/internal/protos"
	"log"
	"maps"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func (s *GrpcDiscoveryServer) Heartbeat(ctx context.Context, in *protos.HeartbeatRequest) (*protos.HeartbeatResponse, error) {
	return &protos.HeartbeatResponse{}, s.updateMembers(in.GetInstances())
}

func (s *GrpcDiscoveryServer) NewMembers(ctx context.Context, in *protos.NewMembersRequest) (*protos.NewMembersResponse, error) {
	for _, instance := range in.Instances {
		s.newMembers(false, instance)
	}
	log.Printf("[GrpcDiscoveryServer/Register] Node[%s] NewMembers complete\n", in.GetInstances())
	return &protos.NewMembersResponse{}, nil
}

func (s *GrpcDiscoveryServer) DelMembers(ctx context.Context, in *protos.DelMembersRequest) (*protos.DelMembersResponse, error) {
	for _, instanceInfo := range in.DelMembers {
		s.delMembers(instanceInfo.GetServiceName(), instanceInfo.GetInstanceId())
	}
	return &protos.DelMembersResponse{}, nil
}

func (s *GrpcDiscoveryServer) SendHeartbeat() {
	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if _, err := s.instances.Heartbeat(context.TODO(), &protos.HeartbeatRequest{Instances: s.instances.Instances}); err != nil {
				log.Println(err)
			}
		case <-s.stopc:
			return
		}
	}
}

func (s *GrpcDiscoveryServer) newMembers(master bool, instance *protos.ServiceInstance) error {
	s.membersrw.Lock()
	if _, ok := s.members[instance.GetServiceName()]; !ok {
		s.members[instance.GetServiceName()] = map[string]*ServiceInstance{}
	}

	newInstance, err := newServiceInstance(s.targetAddr, master, s.heartbeatInterval, instance)
	if err != nil {
		s.membersrw.Unlock()
		return fmt.Errorf("[GrpcDiscoveryServer/NewMembers] NewClient %w", err)
	}
	s.rpcClient.AddStreamClient(instance.GetServiceName(), instance.GetInstanceId(), instance.GetAddr())
	s.members[instance.GetServiceName()][instance.GetInstanceId()] = newInstance
	s.membersrw.Unlock()
	s.remoteHandlerRw.Lock()
	maps.Copy(s.remoteHandler, instance.Messages)
	s.remoteHandlerRw.Unlock()
	return nil
}

func (s *GrpcDiscoveryServer) GetRoute(id uint32) string {
	s.remoteHandlerRw.RLock()
	defer s.remoteHandlerRw.RUnlock()
	return s.remoteHandler[id]
}

func (s *GrpcDiscoveryServer) getMembers(serverName, instanceId string) (*ServiceInstance, bool) {
	serviceInstances, ok := s.members[serverName]
	if !ok {
		return nil, false
	}
	instance, ok := serviceInstances[instanceId]
	return instance, ok
}

func (s *GrpcDiscoveryServer) delMembers(serverName, instanceId string) {
	s.membersrw.Lock()
	defer s.membersrw.Unlock()
	instances, ok := s.members[serverName]
	if !ok {
		return
	}
	instance, ok := instances[instanceId]
	if !ok {
		return
	}
	if !instance.master {
		instance.close()
	}
	delete(s.members[serverName], instanceId)
	s.rpcClient.DelStreamClient(serverName, instanceId)
}

func (s *GrpcDiscoveryServer) updateMembers(ininstance *protos.ServiceInstance) error {
	s.membersrw.Lock()
	defer s.membersrw.Unlock()
	servers, ok := s.members[ininstance.GetServiceName()]
	if !ok {
		s.members[ininstance.GetServiceName()] = map[string]*ServiceInstance{}
	}
	if instance, ok := servers[ininstance.GetInstanceId()]; ok {
		instance.expireAt = time.Now().Add(s.heartbeatInterval)
	} else {
		s.members[ininstance.GetServiceName()][ininstance.GetInstanceId()] = &ServiceInstance{
			Instances: ininstance,
			expireAt:  time.Now().Add(s.heartbeatInterval * 2),
		}
	}
	return nil
}

func (s *GrpcDiscoveryServer) RegisterNode(svrname, addr string, frontend bool, handler map[uint32]string) error {
	var instanceb = make([]byte, 16)
	_, err := crand.Read(instanceb)
	if err != nil {
		return err
	}
	s.instances = &ServiceInstance{
		master: s.targetAddr == "",
		Instances: &protos.ServiceInstance{
			Addr:        addr,
			ServiceName: svrname,
			InstanceId:  hex.EncodeToString(instanceb),
			Frontend:    frontend,
			Messages:    handler,
		},
		expireAt: time.Now().Add(s.heartbeatInterval),
	}
	if _, ok := s.members[svrname]; !ok {
		s.members[svrname] = map[string]*ServiceInstance{}
	}
	s.members[svrname][s.instances.Instances.InstanceId] = s.instances
	if s.instances.master {
		go s.CheckNodeHealth()
		return nil
	}
	conn, err := grpc.NewClient(s.targetAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	s.instances.conn = conn
	s.instances.RegistryServerClient = protos.NewRegistryServerClient(conn)
	_, err = s.instances.Register(context.TODO(), &protos.RegisterRequest{Instances: s.instances.Instances})
	go s.SendHeartbeat()
	return err
}
