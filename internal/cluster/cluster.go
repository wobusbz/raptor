package cluster

import (
	"context"
	"errors"
	"game/internal/clusterpb"
	"log"
	"slices"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type cluster struct {
	currentNode *Node
	rw          sync.RWMutex
	members     []*Member
}

func newCluster(node *Node) *cluster {
	c := &cluster{
		currentNode: node,
		members:     []*Member{},
	}
	if c.currentNode.IsMaster {
		c.checkMemberHeartbeat()
	}
	return c
}

func (c *cluster) Register(_ context.Context, req *clusterpb.RegisterRequest) (*clusterpb.RegisterResponse, error) {

	c.rw.Lock()
	c.members = slices.DeleteFunc(c.members, func(m *Member) bool {
		if m.isMaster {
			return false
		}
		return m.MemberInfo() != nil && m.MemberInfo().GetLabel() == req.GetMemberInfo().GetLabel() && m.MemberInfo().GetServiceAddr() == req.GetMemberInfo().GetServiceAddr()
	})
	c.rw.Unlock()

	var (
		rsp  = &clusterpb.RegisterResponse{}
		errs []error
	)
	newMember := clusterpb.NewMemberRequest{MemberInfo: req.MemberInfo}
	c.rw.RLock()
	for _, m := range c.members {
		if m.MemberInfo().GetLabel() != req.GetMemberInfo().GetLabel() {
			continue
		}
		rsp.Members = append(rsp.Members, m.MemberInfo())
		if m.isMaster {
			continue
		}
		conn, err := c.currentNode.rpcClient.getConnPool(m.MemberInfo().GetServiceAddr())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "服务 [%s] 连接不存在 %v", m.MemberInfo().GetServiceAddr(), err)
		}
		_, err = clusterpb.NewMemberClient(conn.Get()).NewMember(context.Background(), &newMember)
		errs = append(errs, err)
	}
	c.rw.RUnlock()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	c.rw.Lock()
	c.members = append(c.members, &Member{memberinfo: req.MemberInfo, lastHeartbeatAt: time.Now()})
	c.rw.Unlock()
	return rsp, nil
}

func (c *cluster) Unregister(ctx context.Context, req *clusterpb.UnregisterRequest) (*clusterpb.UnregisterResponse, error) {
	if req.GetLabel() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "服务标签不能为空")
	}
	if req.GetServiceAddr() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "服务地址不能为空")
	}
	var errs []error
	c.rw.RLock()
	for _, m := range c.members {
		if m.isMaster {
			continue
		}
		if m.MemberInfo().GetLabel() == req.GetLabel() && m.MemberInfo().GetServiceAddr() == req.GetServiceAddr() {
			continue
		}
		conn, err := c.currentNode.rpcClient.getConnPool(m.MemberInfo().GetServiceAddr())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "服务 [%s] 连接不存在 %v", m.MemberInfo().GetServiceAddr(), err)
		}
		_, err = clusterpb.NewMemberClient(conn.Get()).DelMember(ctx, &clusterpb.DelMemberRequest{Label: req.GetLabel(), ServiceAddr: req.GetServiceAddr()})
		errs = append(errs, err)
	}
	c.rw.RUnlock()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	c.rw.Lock()
	c.members = slices.DeleteFunc(c.members, func(m *Member) bool {
		return m.MemberInfo().GetLabel() == req.GetLabel() && m.MemberInfo().GetServiceAddr() == req.GetServiceAddr()
	})
	c.rw.Unlock()
	return &clusterpb.UnregisterResponse{}, nil
}

func (c *cluster) Heartbeat(_ context.Context, req *clusterpb.HeartbeatRequest) (*clusterpb.HeartbeatResponse, error) {
	if req.GetMemberInfo() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "MemberInfo 成员信息为空")
	}
	var isexist bool
	c.rw.Lock()
	for _, m := range c.members {
		if m.isMaster {
			continue
		}
		if m.MemberInfo().GetLabel() != req.GetMemberInfo().GetLabel() {
			continue
		}
		if m.MemberInfo().GetServiceAddr() != req.GetMemberInfo().GetServiceAddr() {
			continue
		}
		m.lastHeartbeatAt = time.Now()
		isexist = true
		break
	}
	if !isexist {
		c.members = append(c.members, &Member{memberinfo: req.GetMemberInfo(), lastHeartbeatAt: time.Now()})
	}
	c.rw.Unlock()
	return &clusterpb.HeartbeatResponse{}, nil
}

func (c *cluster) checkMemberHeartbeat() {
	check := func(now time.Time) {
		var unregisterMembers = make([]*Member, 0)
		c.rw.RLock()
		for _, m := range c.members {
			if m.isMaster {
				continue
			}
			if now.Sub(m.lastHeartbeatAt) < time.Minute {
				continue
			}
			unregisterMembers = append(unregisterMembers, m)
		}
		c.rw.RUnlock()

		var req = &clusterpb.UnregisterRequest{}

		for _, unm := range unregisterMembers {
			req.Label = unm.MemberInfo().GetLabel()
			req.ServiceAddr = unm.MemberInfo().GetServiceAddr()
			if _, err := c.Unregister(context.Background(), req); err != nil {
				log.Println("Heartbeat unregister error", err)
			}
		}
	}
	go func() {
		ticker := time.NewTicker(time.Second * 10)
		defer ticker.Stop()
		for {
			select {
			case tm := <-ticker.C:
				check(tm)
			case <-c.currentNode.ctx.Done():
				return
			}
		}
	}()
}

func (c *cluster) addMemberInfo(info *clusterpb.MemberInfo) {
	var isexist bool

	c.rw.Lock()
	for _, m := range c.members {
		if m.MemberInfo().GetLabel() != info.GetLabel() {
			continue
		}
		if m.MemberInfo().GetServiceAddr() != info.GetServiceAddr() {
			continue
		}
		m.memberinfo = info
		isexist = true
		break
	}
	if !isexist {
		c.members = append(c.members, &Member{memberinfo: info, lastHeartbeatAt: time.Now()})
	}
	c.rw.Unlock()
}

func (c *cluster) delMemberInfo(info *clusterpb.DelMemberRequest) {
	c.rw.Lock()
	c.members = slices.DeleteFunc(c.members, func(m *Member) bool {
		return m.MemberInfo().GetLabel() == info.GetLabel() && m.MemberInfo().GetServiceAddr() == info.GetServiceAddr()
	})
	c.rw.Unlock()
}
