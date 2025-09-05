package cluster

import (
	"errors"
	"fmt"
	"game/internal/protos"
	"game/session"
	"math/rand"
)

func (s *GrpcDiscoveryServer) SelectNode() (map[string]*ServiceInstance, bool) {
	s.membersrw.RLock()
	defer s.membersrw.RUnlock()
	var result = map[string]*ServiceInstance{}
	for name, members := range s.members {
		if s.instances.Instances.ServiceName == name {
			continue
		}
		randomInstances := make([]*ServiceInstance, 0)
		for _, mem := range members {
			randomInstances = append(randomInstances, mem)
		}
		result[name] = randomInstances[rand.Int()%len(randomInstances)]
	}
	return result, true
}

func (s *GrpcDiscoveryServer) NotifyOnSession(session session.Session) error {
	var errs []error
	members, _ := s.SelectNode()
	instances := map[string]*protos.ServiceInstance{
		s.instances.Instances.ServiceName: s.instances.Instances,
	}
	for _, mem := range members {
		instances[mem.Instances.ServiceName] = mem.Instances
		session.Routers().BindServer(mem.Instances.ServiceName, mem.Instances.InstanceId)
	}
	for _, mem := range members {
		streamClient, err := s.rpcClient.GetStreamClient(
			mem.Instances.GetServiceName(),
			mem.Instances.GetInstanceId(),
			mem.Instances.GetAddr(),
		)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := streamClient.Send(&protos.RemoteMessage{
			Kind:                    protos.RemoteMessage_KIND_ON_SESSION_CONNECT,
			OnSessionConnectMessage: &protos.OnSessionConnectMessage{ID: session.ID(), Instances: instances},
		}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *GrpcDiscoveryServer) NotifyOnSessionClose(session session.Session) error {
	var errs []error
	remoteMessage := &protos.RemoteMessage{
		Kind:      protos.RemoteMessage_KIND_ON_SESSION_DISCONNECT,
		SessionID: session.ID(),
	}
	for svrname, instanceId := range session.Routers().Routers() {
		svrNodeInfo, ok := s.GetNodeInfo(svrname, instanceId)
		if !ok {
			errs = append(errs, fmt.Errorf("[RemoteHandler/notifyRemoteOnSessionClose] server %s not found", svrname))
			continue
		}
		rpcClient, err := s.rpcClient.GetStreamClient(svrname, instanceId, svrNodeInfo.Instances.Addr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		errs = append(errs, rpcClient.Send(remoteMessage))
	}
	return errors.Join(errs...)
}

func (s *GrpcDiscoveryServer) GetNodeInfo(svrname, instanceId string) (*ServiceInstance, bool) {
	s.membersrw.RLock()
	defer s.membersrw.RUnlock()
	instances, ok := s.members[svrname]
	if !ok {
		return nil, false
	}
	for _, v := range instances {
		if v.Instances.InstanceId != instanceId {
			continue
		}
		return v, true
	}
	return nil, false
}
