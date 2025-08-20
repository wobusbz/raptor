package cluster

import (
	"context"
	"game/internal/clusterpb"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
)

type Options struct {
	IsMaster           bool
	ServerAddr         string
	AdvertiseAddr      string
	RetryInterval      time.Duration
	ClientAddr         string
	Label              string
	UnregisterCallback func(Member)
}

type Node struct {
	Options
	ctx           context.Context
	cancel        context.CancelFunc
	cluster       *cluster
	server        *grpc.Server
	rpcClient     *rpcClient
	keepaliveExit chan struct{}
}

func NewNode(opts ...Options) *Node {
	return &Node{}
}

func (n *Node) Startup() error {
	n.cluster = newCluster(n)
	return n.initNode()
}

func (n *Node) initNode() error {
	n.rpcClient = newRPCClient()
	n.ctx, n.cancel = context.WithCancel(context.Background())

	if n.IsMaster {
		listen, err := net.Listen("tcp", "0.0.0.0:9910")
		if err != nil {
			return err
		}
		n.server = grpc.NewServer()
		go func() {
			if err = n.server.Serve(listen); err != nil {
				log.Fatalf("Error Start current node failed %v", err)
			}
		}()
		clusterpb.RegisterMasterServer(n.server, n.cluster)
		member := &Member{
			isMaster: true,
			memberinfo: &clusterpb.MemberInfo{
				Label:       n.Label,
				ServiceAddr: "",
			},
		}
		n.cluster.members = append(n.cluster.members, member)
	} else {
		conn, err := n.rpcClient.getConnPool(n.AdvertiseAddr)
		if err != nil {
			return err
		}
		clusterpb.NewMasterClient(conn.Get()).Register(n.ctx, nil)
	}
	return nil
}

func (n *Node) Shutdown() error {
	n.server.GracefulStop()
	return nil
}

func (n *Node) HandleRequest(context.Context, *clusterpb.RequestMessage) (*clusterpb.MemberHandleResponse, error) {
	return &clusterpb.MemberHandleResponse{}, nil
}

func (n *Node) HandleNotify(context.Context, *clusterpb.NotifyMessage) (*clusterpb.MemberHandleResponse, error) {
	return &clusterpb.MemberHandleResponse{}, nil
}

func (n *Node) HandlePush(context.Context, *clusterpb.PushMessage) (*clusterpb.MemberHandleResponse, error) {
	return &clusterpb.MemberHandleResponse{}, nil
}

func (n *Node) HandleResponse(context.Context, *clusterpb.ResponseMessage) (*clusterpb.MemberHandleResponse, error) {
	return &clusterpb.MemberHandleResponse{}, nil
}

func (n *Node) NewMember(_ context.Context, req *clusterpb.NewMemberRequest) (*clusterpb.NewMemberResponse, error) {
	n.cluster.addMemberInfo(req.MemberInfo)
	return &clusterpb.NewMemberResponse{}, nil
}

func (n *Node) DelMember(_ context.Context, req *clusterpb.DelMemberRequest) (*clusterpb.DelMemberResponse, error) {
	n.cluster.delMemberInfo(req)
	return &clusterpb.DelMemberResponse{}, nil
}

func (n *Node) SessionClosed(context.Context, *clusterpb.SessionClosedRequest) (*clusterpb.SessionClosedResponse, error) {
	return &clusterpb.SessionClosedResponse{}, nil
}

func (n *Node) CloseSession(context.Context, *clusterpb.CloseSessionRequest) (*clusterpb.CloseSessionResponse, error) {
	return &clusterpb.CloseSessionResponse{}, nil
}

func (n *Node) keepalive() {
	if n.AdvertiseAddr == "" || n.IsMaster {
		return
	}
	heartbeat := func() {
		pool, err := n.rpcClient.getConnPool(n.AdvertiseAddr)
		if err != nil {
			log.Println("rpcClient master conn", err)
			return
		}
		masterCli := clusterpb.NewMasterClient(pool.Get())
		if _, err := masterCli.Heartbeat(context.Background(), &clusterpb.HeartbeatRequest{
			MemberInfo: &clusterpb.MemberInfo{
				Label: n.Label,
			},
		}); err != nil {
			log.Println("Member send heartbeat error", err)
		}
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				heartbeat()
			case <-n.keepaliveExit:
				log.Println("Exit member node heartbeat ")
				ticker.Stop()
				return
			}
		}
	}()
}
