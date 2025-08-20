package cluster

import (
	"game/internal/clusterpb"
	"time"
)

type Member struct {
	isMaster        bool
	memberinfo      *clusterpb.MemberInfo
	lastHeartbeatAt time.Time
}

func (m *Member) MemberInfo() *clusterpb.MemberInfo {
	return m.memberinfo
}
