package cluster

import (
	"context"
	"errors"
	"fmt"
	"game/internal/protos"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcDiscoveryServer struct {
	protos.UnimplementedRegistryServerServer
	grpcDiscoveryClient *grpcDiscoveryClient
	stopc               chan struct{}
	rpcClinet           *rpcClient
	closeonce           sync.Once
}

func NewGRPCDiscoveryServer(grpcgrpcDiscoveryClient *grpcDiscoveryClient, rpcClient *rpcClient) *grpcDiscoveryServer {
	gs := &grpcDiscoveryServer{
		grpcDiscoveryClient: grpcgrpcDiscoveryClient,
		stopc:               make(chan struct{}),
		rpcClinet:           rpcClient,
	}
	gs.checkMemberHeartbeat()
	return gs
}

func (gs *grpcDiscoveryServer) Register(ctx context.Context, in *protos.RegisterRequest) (*protos.RegisterResponse, error) {
	gs.grpcDiscoveryClient.newMembers(false, in.GetInstances())

	members := gs.grpcDiscoveryClient.getAllMembers()

	for _, serviceInstances := range members {
		for _, s := range serviceInstances {
			if s.Instances.GetServiceName() == in.Instances.GetServiceName() {
				continue
			}
			if s.Instances.GetInstanceId() == in.Instances.GetInstanceId() {
				continue
			}
			rpcconn, err := gs.rpcClinet.getConnPool(s.Instances.GetAddr())
			if err != nil {
				return nil, status.Errorf(codes.Internal, "[Register] 获取旧节点 服务 %s 实例 %s 连接错误: %v", s.Instances.GetServiceName(), s.Instances.GetInstanceId(), err)
			}
			_, err = protos.NewMembersServerClient(rpcconn.Get()).NewMembers(context.TODO(), &protos.NewMembersRequest{Instances: in.Instances})
			if err != nil {
				return nil, status.Errorf(codes.Internal, "[Register] 通知旧节点 服务 %s 实例 %s -> 新节点 %s 同步失败: %v", s.Instances.GetServiceName(), s.Instances.GetInstanceId(), in.Instances.GetInstanceId(), err)
			}
			rpcconnnew, err := gs.rpcClinet.getConnPool(in.Instances.GetAddr())
			if err != nil {
				return nil, status.Errorf(codes.Internal, "[Register] 获取新节点 服务 %s 实例 %s 连接错误: %v", in.Instances.GetServiceName(), in.Instances.GetInstanceId(), err)
			}
			_, err = protos.NewMembersServerClient(rpcconnnew.Get()).NewMembers(context.TODO(), &protos.NewMembersRequest{Instances: s.Instances})
			if err != nil {
				return nil, status.Errorf(codes.Internal, "[Register] 通知新节点 服务 %s 实例 %s <- 旧节点 %s 同步失败: %v", in.Instances.GetServiceName(), in.Instances.GetInstanceId(), s.Instances.GetInstanceId(), err)
			}
		}
	}
	// 主节点同步
	rpcconnnew, err := gs.rpcClinet.getConnPool(in.Instances.GetAddr())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "[Register] 主节点同步: 获取新节点 服务 %s 实例 %s 连接错误: %v", in.Instances.GetServiceName(), in.Instances.GetInstanceId(), err)
	}
	if _, err = protos.NewMembersServerClient(rpcconnnew.Get()).NewMembers(context.TODO(), &protos.NewMembersRequest{
		Instances: gs.grpcDiscoveryClient.getCurServerInstance(),
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "[Register] 主节点同步: 新节点 %s <- 主节点同步失败: %v", in.Instances.GetInstanceId(), err)
	}
	return &protos.RegisterResponse{}, nil
}

func (gs *grpcDiscoveryServer) Deregister(ctx context.Context, in *protos.DeregisterRequest) (*protos.DeregisterResponse, error) {
	gs.grpcDiscoveryClient.delMembers(in.Instances.GetServiceName(), in.Instances.GetInstanceId())

	var errs []error
	for _, serviceInstances := range gs.grpcDiscoveryClient.getAllMembers() {
		for _, s := range serviceInstances {
			if s.Instances.GetServiceName() == in.Instances.GetServiceName() {
				continue
			}
			if s.Instances.GetInstanceId() == in.Instances.GetInstanceId() {
				continue
			}
			rpcconn, err := gs.rpcClinet.getConnPool(s.Instances.GetAddr())
			if err != nil {
				errs = append(errs, fmt.Errorf("[Deregister] 获取节点 服务 %s 实例 %s 连接错误: %v", s.Instances.GetServiceName(), s.Instances.GetInstanceId(), err))
				continue
			}
			_, err = protos.NewMembersServerClient(rpcconn.Get()).DelMembers(ctx, &protos.DelMembersRequest{
				InstanceId:  in.Instances.GetInstanceId(),
				ServiceName: in.Instances.GetServiceName(),
			})
			if err != nil {
				errs = append(errs, fmt.Errorf("[Deregister] 通知节点 服务 %s 实例 %s 删除成员 %s 错误: %v", s.Instances.GetServiceName(), s.Instances.GetInstanceId(), in.Instances.GetInstanceId(), err))
			}
		}
	}
	return &protos.DeregisterResponse{}, errors.Join(errs...)
}

func (gs *grpcDiscoveryServer) Discover(ctx context.Context, in *protos.DiscoveryRequest) (*protos.DiscoveryResponse, error) {
	var instances []*protos.ServiceInstance
	for _, s := range gs.grpcDiscoveryClient.getMembers(in.GetServiceName()) {
		instances = append(instances, s.Instances)
	}
	return &protos.DiscoveryResponse{Instances: instances}, nil
}

func (gs *grpcDiscoveryServer) Heartbeat(ctx context.Context, in *protos.HeartbeatRequest) (*protos.HeartbeatResponse, error) {
	return &protos.HeartbeatResponse{}, gs.grpcDiscoveryClient.updateMembers(in.GetServiceName(), in.GetInstanceId())
}

func (gs *grpcDiscoveryServer) checkMemberHeartbeat() {
	check := func(now time.Time) {
		var unregisterMembers = make([]*protos.ServiceInstance, 0)
		for _, serviceInstances := range gs.grpcDiscoveryClient.getAllMembers() {
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
		for _, s := range unregisterMembers {
			_, _ = gs.Deregister(context.TODO(), &protos.DeregisterRequest{Instances: s})
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
			case <-gs.stopc:
				return
			}
		}
	}()
}

func (gs *grpcDiscoveryServer) Shutdown() {
	gs.closeonce.Do(func() { close(gs.stopc) })
}
