package cluster

import (
	"context"
	"errors"
	"fmt"
	"game/internal/protos"
	"log"
	"slices"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	ServiceDiscovery interface {
		SelectNode() *ServiceInstance
		GetNodeInfo(svrname, instanceId string) (*ServiceInstance, bool)
	}

	ServiceInstance struct {
		Instances *protos.ServiceInstance
		expireAt  time.Time
		master    bool
	}
	GrpcDiscoveryServer struct {
		protos.UnimplementedRegistryServerServer
		instances         *ServiceInstance
		rpcClient         *rpcClient
		members           map[string][]*ServiceInstance
		membersrw         sync.RWMutex
		heartbeatInterval time.Duration
		heartbeatTimeout  time.Duration
		stopc             chan struct{}
		rpcClinet         *rpcClient
		closeonce         sync.Once
	}
)

func NewGRPCDiscoveryServer(rpcClient *rpcClient) *GrpcDiscoveryServer {
	gs := &GrpcDiscoveryServer{
		stopc:     make(chan struct{}),
		rpcClinet: rpcClient,
	}
	gs.checkMemberHeartbeat()
	return gs
}

func (s *GrpcDiscoveryServer) Register(ctx context.Context, in *protos.RegisterRequest) (*protos.RegisterResponse, error) {
	s.newMembers(false, in.GetInstances())

	members := s.getAllMembers()

	for _, serviceInstances := range members {
		for _, instance := range serviceInstances {
			if instance.Instances.GetServiceName() == in.Instances.GetServiceName() {
				continue
			}
			if instance.Instances.GetInstanceId() == in.Instances.GetInstanceId() {
				continue
			}
			rpcconn, err := s.rpcClinet.getConnPool(instance.Instances.GetAddr())
			if err != nil {
				return nil, status.Errorf(codes.Internal, "[Register] 获取旧节点 服务 %s 实例 %s 连接错误: %v", instance.Instances.GetServiceName(), instance.Instances.GetInstanceId(), err)
			}
			_, err = protos.NewMembersServerClient(rpcconn.Get()).NewMembers(context.TODO(), &protos.NewMembersRequest{Instances: in.Instances})
			if err != nil {
				return nil, status.Errorf(codes.Internal, "[Register] 通知旧节点 服务 %s 实例 %s -> 新节点 %s 同步失败: %v", instance.Instances.GetServiceName(), instance.Instances.GetInstanceId(), in.Instances.GetInstanceId(), err)
			}
			rpcconnnew, err := s.rpcClinet.getConnPool(in.Instances.GetAddr())
			if err != nil {
				return nil, status.Errorf(codes.Internal, "[Register] 获取新节点 服务 %s 实例 %s 连接错误: %v", in.Instances.GetServiceName(), in.Instances.GetInstanceId(), err)
			}
			_, err = protos.NewMembersServerClient(rpcconnnew.Get()).NewMembers(context.TODO(), &protos.NewMembersRequest{Instances: instance.Instances})
			if err != nil {
				return nil, status.Errorf(codes.Internal, "[Register] 通知新节点 服务 %s 实例 %s <- 旧节点 %s 同步失败: %v", in.Instances.GetServiceName(), in.Instances.GetInstanceId(), instance.Instances.GetInstanceId(), err)
			}
		}
	}
	// 主节点同步
	rpcconnnew, err := s.rpcClinet.getConnPool(in.Instances.GetAddr())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "[Register] 主节点同步: 获取新节点 服务 %s 实例 %s 连接错误: %v", in.Instances.GetServiceName(), in.Instances.GetInstanceId(), err)
	}
	if _, err = protos.NewMembersServerClient(rpcconnnew.Get()).NewMembers(context.TODO(), &protos.NewMembersRequest{
		Instances: s.instances.Instances,
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "[Register] 主节点同步: 新节点 %s <- 主节点同步失败: %v", in.Instances.GetInstanceId(), err)
	}
	return &protos.RegisterResponse{}, nil
}

func (s *GrpcDiscoveryServer) Deregister(ctx context.Context, in *protos.DeregisterRequest) (*protos.DeregisterResponse, error) {
	s.delMembers(in.Instances.GetServiceName(), in.Instances.GetInstanceId())

	var errs []error
	for _, serviceInstances := range s.getAllMembers() {
		for _, instance := range serviceInstances {
			if instance.Instances.GetServiceName() == in.Instances.GetServiceName() {
				continue
			}
			if instance.Instances.GetInstanceId() == in.Instances.GetInstanceId() {
				continue
			}
			rpcconn, err := s.rpcClinet.getConnPool(instance.Instances.GetAddr())
			if err != nil {
				errs = append(errs, fmt.Errorf("[Deregister] 获取节点 服务 %s 实例 %s 连接错误: %v", instance.Instances.GetServiceName(), instance.Instances.GetInstanceId(), err))
				continue
			}
			_, err = protos.NewMembersServerClient(rpcconn.Get()).DelMembers(ctx, &protos.DelMembersRequest{
				InstanceId:  in.Instances.GetInstanceId(),
				ServiceName: in.Instances.GetServiceName(),
			})
			if err != nil {
				errs = append(errs, fmt.Errorf("[Deregister] 通知节点 服务 %s 实例 %s 删除成员 %s 错误: %v", instance.Instances.GetServiceName(), instance.Instances.GetInstanceId(), in.Instances.GetInstanceId(), err))
			}
		}
	}
	return &protos.DeregisterResponse{}, errors.Join(errs...)
}

func (s *GrpcDiscoveryServer) Heartbeat(ctx context.Context, in *protos.HeartbeatRequest) (*protos.HeartbeatResponse, error) {
	return &protos.HeartbeatResponse{}, s.updateMembers(in.GetInstances())
}

func (s *GrpcDiscoveryServer) checkMemberHeartbeat() {
	check := func(now time.Time) {
		var unregisterMembers = make([]*protos.ServiceInstance, 0)
		for _, serviceInstances := range s.getAllMembers() {
			for _, s := range serviceInstances {
				if s.master {
					continue
				}
				if now.Before(s.expireAt) {
					continue
				}
				unregisterMembers = append(unregisterMembers, s.Instances)
			}
		}
		for _, instance := range unregisterMembers {
			_, _ = s.Deregister(context.TODO(), &protos.DeregisterRequest{Instances: instance})
			log.Println(s)
		}
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case tm := <-ticker.C:
				check(tm)
			case <-s.stopc:
				return
			}
		}
	}()
}

func (s *GrpcDiscoveryServer) NewMembers(ctx context.Context, in *protos.NewMembersRequest) (*protos.NewMembersResponse, error) {
	s.newMembers(false, in.GetInstances())
	go s.heartbeat(in.GetInstances().GetAddr())
	return &protos.NewMembersResponse{}, nil
}

func (s *GrpcDiscoveryServer) DelMembers(ctx context.Context, in *protos.DelMembersRequest) (*protos.DelMembersResponse, error) {
	s.delMembers(in.GetServiceName(), in.GetInstanceId())
	return &protos.DelMembersResponse{}, nil
}

func (s *GrpcDiscoveryServer) heartbeat(targetAddr string) {
	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rpcconn, err := s.rpcClient.getConnPool(targetAddr)
			if err != nil {
				log.Println("heartbeat getConnPool error:", err)
				continue
			}
			_, err = protos.NewRegistryServerClient(rpcconn.Get()).Heartbeat(context.TODO(), &protos.HeartbeatRequest{
				Instances: s.instances.Instances,
			})
			if err != nil {
				log.Println("heartbeat send error:", err)
			}
		case <-s.stopc:
			return
		}
	}
}

func (s *GrpcDiscoveryServer) newMembers(master bool, instance *protos.ServiceInstance) {
	s.membersrw.Lock()
	servers, ok := s.members[instance.GetServiceName()]
	if ok {
		s.members[instance.GetServiceName()] = slices.DeleteFunc(servers, func(s *ServiceInstance) bool {
			return s.Instances.GetInstanceId() == instance.GetInstanceId()
		})
	}
	s.members[instance.GetServiceName()] = append(s.members[instance.GetServiceName()], &ServiceInstance{
		master:    master,
		Instances: instance,
		expireAt:  time.Now().Add(s.heartbeatTimeout),
	})
	s.membersrw.Unlock()
}

func (s *GrpcDiscoveryServer) delMembers(serverName, instanceId string) {
	s.membersrw.Lock()
	servers, ok := s.members[serverName]
	if ok {
		s.members[serverName] = slices.DeleteFunc(servers, func(s *ServiceInstance) bool {
			return s.Instances.GetInstanceId() == instanceId
		})
	}
	s.membersrw.Unlock()
}

func (s *GrpcDiscoveryServer) getAllMembers() map[string][]*ServiceInstance {
	s.membersrw.RLock()
	defer s.membersrw.RUnlock()
	out := make(map[string][]*ServiceInstance, len(s.members))
	for k, v := range s.members {
		cp := make([]*ServiceInstance, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

func (s *GrpcDiscoveryServer) updateMembers(ininstance *protos.ServiceInstance) error {
	s.membersrw.Lock()
	defer s.membersrw.Unlock()
	servers, ok := s.members[ininstance.GetServiceName()]
	if !ok {
		s.members[ininstance.GetServiceName()] = append(s.members[ininstance.GetServiceName()], &ServiceInstance{
			Instances: ininstance,
			expireAt:  time.Now().Add(s.heartbeatTimeout),
		})
	} else {
		for _, instance := range servers {
			if instance.Instances.GetInstanceId() != ininstance.GetInstanceId() {
				continue
			}
			instance.expireAt = time.Now().Add(s.heartbeatTimeout)
		}
	}
	return nil
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

func (s *GrpcDiscoveryServer) Shutdown() {
	s.closeonce.Do(func() { close(s.stopc) })
}
