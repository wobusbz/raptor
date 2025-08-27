package session

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
)

type SessionMessage interface {
	GetServerName() string
	GetMsgCode() int32
	GetMsgName() string
	GetModelName() string
	proto.Message
}

type MsgMeta struct {
	Server string
	Model  string
	Name   string
	Code   int32
}

func (m *MsgMeta) Router() string {
	return fmt.Sprintf("%s/%s/%s", m.Server, m.Model, m.Name)
}

type MetadataProvider interface {
	Extract(proto.Message) (*MsgMeta, error)
}

type DirectMetadataProvider struct{}

func (DirectMetadataProvider) Extract(m proto.Message) (*MsgMeta, error) {
	sm, ok := m.(SessionMessage)
	if !ok {
		return nil, errors.New("invalid message: no metadata")
	}
	return &MsgMeta{Server: sm.GetServerName(), Model: sm.GetModelName(), Name: sm.GetMsgName(), Code: sm.GetMsgCode()}, nil
}
