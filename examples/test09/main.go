package main

import (
	"fmt"
	"game/internal/protos"
	"reflect"

	"google.golang.org/protobuf/proto"
)

type Message interface {
	GetServerName() string
	GetMsgCode() int32
	GetMsgName() string
	proto.Message
}

type CallRequest struct {
	*protos.DelMembersRequest
}

func (c *CallRequest) GetServerName() string { return "GAME" }
func (c *CallRequest) GetMsgCode() int32     { return 1 }
func (c *CallRequest) GetMsgName() string    { return "CallRequest" }

func isHandlerMethod(method reflect.Method) bool {
	mt := method.Type
	if method.PkgPath != "" {
		return false
	}
	fmt.Println(mt.NumIn())
	if mt.NumIn() != 2 && mt.NumIn() != 3 {
		return false
	}
	fmt.Println(int(mt.In(2).Kind()), mt.NumIn())
	fmt.Println(mt.NumIn() == 3 && mt.In(2).Kind() != reflect.Pointer, mt.NumOut())

	return true
}

type User struct {
}

func (u *User) LoadUser(int, *CallRequest) {

}

func main() {
	var msgcode = map[int32]Message{
		1: (*CallRequest)(nil),
	}
	fmt.Println(msgcode[1].GetServerName())
	fmt.Println(msgcode[1].GetMsgName())
}
