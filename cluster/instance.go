package cluster

import (
	"fmt"
	"game/internal/protos"
	"game/session"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ServiceDiscovery interface {
	Name() string
	SelectNode() (map[string]*ServiceInstance, bool)
	GetNodeInfo(svrname, instanceId string) (*ServiceInstance, bool)
	NotifyOnSession(session session.Session) error
	NotifyOnSessionClose(session session.Session) error
	GetRoute(id uint32) string
}

type ServiceInstance struct {
	protos.RegistryServerClient
	Instances *protos.ServiceInstance
	expireAt  time.Time
	master    bool
	conn      *grpc.ClientConn
}

func newServiceInstance(targetAddr string, master bool, heartbeatInterval time.Duration, instance *protos.ServiceInstance) (*ServiceInstance, error) {
	if !master {
		targetAddr = instance.GetAddr()
	}
	conn, err := grpc.NewClient(instance.GetAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &ServiceInstance{
		master:               master,
		Instances:            instance,
		expireAt:             time.Now().Add(heartbeatInterval),
		conn:                 conn,
		RegistryServerClient: protos.NewRegistryServerClient(conn),
	}, nil
}

func (s *ServiceInstance) String() string {
	return fmt.Sprintf("SericeName: %s InstanceId: %s Frontend: %v Addr: %s",
		s.Instances.ServiceName,
		s.Instances.InstanceId,
		s.Instances.Frontend,
		s.Instances.Addr,
	)
}

func (s *ServiceInstance) close() error {
	if s.conn == nil {
		return nil
	}
	s.RegistryServerClient = nil
	s.Instances = nil
	return s.conn.Close()
}
