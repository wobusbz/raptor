package cluster

import (
	"context"
	"errors"
	"fmt"
	"game/internal/protos"
	"game/stream"
	"log"
	"sync"
	"time"
)

type GrpcDiscoveryServer struct {
	protos.UnimplementedRegistryServerServer
	targetAddr        string
	instances         *ServiceInstance
	rpcClient         *stream.StreamClientManager
	members           map[string]map[string]*ServiceInstance
	membersrw         sync.RWMutex
	heartbeatInterval time.Duration
	stopc             chan struct{}
	closeonce         sync.Once
	remoteHandler     map[uint32]string
	remoteHandlerRw   sync.RWMutex
}

func NewGRPCDiscoveryServer(targetAddr string, rpcClient *stream.StreamClientManager) *GrpcDiscoveryServer {
	return &GrpcDiscoveryServer{
		targetAddr:        targetAddr,
		rpcClient:         rpcClient,
		heartbeatInterval: time.Second * 5,
		members:           map[string]map[string]*ServiceInstance{},
		stopc:             make(chan struct{}),
		remoteHandler:     make(map[uint32]string),
	}
}

func (s *GrpcDiscoveryServer) Name() string {
	return s.instances.Instances.ServiceName
}

func (s *GrpcDiscoveryServer) Register(ctx context.Context, in *protos.RegisterRequest) (*protos.RegisterResponse, error) {
	var newMembers = make([]*protos.ServiceInstance, 0, len(s.members))
	s.membersrw.RLock()
	for sname, serviceInstances := range s.members {
		if sname == in.Instances.GetServiceName() {
			continue
		}
		for _, instance := range serviceInstances {
			newMembers = append(newMembers, instance.Instances)
			if instance.master {
				continue
			}
			if _, err := instance.NewMembers(ctx, &protos.NewMembersRequest{
				Instances: []*protos.ServiceInstance{in.Instances},
			}); err != nil {
				s.membersrw.RUnlock()
				log.Printf("[GrpcDiscoveryServer/Register] 通知旧节点 服务 %s 实例 %s -> 新节点 %s 同步失败: %v \n",
					instance.Instances.GetServiceName(),
					instance.Instances.GetInstanceId(),
					instance.Instances.GetInstanceId(),
					err,
				)
				return nil, err
			}
		}
	}
	s.membersrw.RUnlock()
	if err := s.newMembers(false, in.GetInstances()); err != nil {
		log.Printf("[GrpcDiscoveryServer/Register] NewMembers InstanceId: %s ServiceName: %v failed %v\n",
			in.GetInstances().GetInstanceId(),
			in.GetInstances().GetServiceName(),
			err,
		)
		return nil, err
	}
	instances, ok := s.getMembers(in.GetInstances().GetServiceName(), in.GetInstances().GetInstanceId())
	if !ok {
		return nil, fmt.Errorf("[GrpcDiscoveryServer/Register] svrname:%s instanceId:%s not found", in.GetInstances().GetServiceName(), in.GetInstances().GetInstanceId())
	}
	_, err := instances.NewMembers(ctx, &protos.NewMembersRequest{Instances: newMembers})
	log.Printf("[GrpcDiscoveryServer/Register] Node[%s] Register complete\n", s.instances.String())
	return &protos.RegisterResponse{}, err
}

func (s *GrpcDiscoveryServer) Deregister(ctx context.Context, in *protos.DeregisterRequest) (*protos.DeregisterResponse, error) {
	s.delMembers(in.Instances.GetServiceName(), in.Instances.GetInstanceId())

	var delMember = []*protos.DelMembers{{InstanceId: in.Instances.GetInstanceId(), ServiceName: in.Instances.GetServiceName()}}
	s.membersrw.RLock()
	defer s.membersrw.RUnlock()
	var errs []error
	for sname, serviceInstances := range s.members {
		if sname == in.Instances.GetServiceName() {
			continue
		}
		for _, instance := range serviceInstances {
			if instance.master {
				continue
			}
			if _, err := instance.DelMembers(ctx, &protos.DelMembersRequest{DelMembers: delMember}); err != nil {
				errs = append(errs, fmt.Errorf("[Deregister] 通知节点 服务 %s 实例 %s 删除成员 %s 错误: %v", instance.Instances.GetServiceName(), instance.Instances.GetInstanceId(), in.Instances.GetInstanceId(), err))
			}
		}
	}
	return &protos.DeregisterResponse{}, errors.Join(errs...)
}

func (s *GrpcDiscoveryServer) CheckNodeHealth() {
	checkNodeHealth := func(vnow int64) {
		removeNodes := make([]*ServiceInstance, 0)
		notifyNodes := make([]*ServiceInstance, 0)
		s.membersrw.RLock()
		for _, serverInstances := range s.members {
			for _, instance := range serverInstances {
				if instance.master {
					continue
				}
				if instance.expireAt.Unix() < vnow {
					removeNodes = append(removeNodes, instance)
				} else {
					notifyNodes = append(notifyNodes, instance)
				}
			}
		}
		s.membersrw.RUnlock()
		if len(removeNodes) == 0 {
			return
		}
		var delMembers = make([]*protos.DelMembers, 0, len(removeNodes))
		for _, instance := range removeNodes {
			s.delMembers(instance.Instances.GetServiceName(), instance.Instances.GetInstanceId())
			delMembers = append(delMembers, &protos.DelMembers{
				ServiceName: instance.Instances.GetServiceName(),
				InstanceId:  instance.Instances.GetInstanceId(),
			})
		}
		for _, instance := range notifyNodes {
			instance.DelMembers(context.TODO(), &protos.DelMembersRequest{DelMembers: delMembers})
		}
	}
	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case vunix := <-ticker.C:
			checkNodeHealth(vunix.Unix())
		case <-s.stopc:
			return
		}
	}
}

func (s *GrpcDiscoveryServer) Shutdown() {
	s.closeonce.Do(func() {
		close(s.stopc)
		if s.instances.master {
			return
		}
		s.instances.Deregister(context.TODO(), &protos.DeregisterRequest{Instances: s.instances.Instances})
	})
}
