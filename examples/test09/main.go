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
	*protos.CallRequest
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
	// if t1 := mt.In(1); !t1.Implements(typeOfContext) {
	// 	return false
	// }
	fmt.Println(int(mt.In(2).Kind()), mt.NumIn())
	fmt.Println(mt.NumIn() == 3 && mt.In(2).Kind() != reflect.Pointer, mt.NumOut())

	// if mt.NumIn() == 3 && mt.In(2).Kind() != reflect.Ptr && mt.In(2) != typeOfBytes {
	// 	return false
	// }
	// if mt.NumOut() != 0 && mt.NumOut() != 2 {
	// 	return false
	// }
	// if mt.NumOut() == 2 && (mt.Out(1) != typeOfError || mt.Out(0) != typeOfBytes && mt.Out(0).Kind() != reflect.Ptr) {
	// 	return false
	// }

	return true
}

type User struct {
}

func (u *User) LoadUser(int, *CallRequest) {

}

func main() {
	proto.Marshal(&CallRequest{
		CallRequest: &protos.CallRequest{
			Data: []byte("hello world"),
		},
	})
}

// func suitableHandlerMethods(typ reflect.Type, nameFunc func(string) string) map[string]*Handler {
// 	methods := make(map[string]*Handler)
// 	for m := 0; m < typ.NumMethod(); m++ {
// 		method := typ.Method(m)
// 		mt := method.Type
// 		mn := method.Name
// 		if isHandlerMethod(method) {
// 			raw := false
// 			if mt.NumIn() == 3 && mt.In(2) == typeOfBytes {
// 				raw = true
// 			}
// 			// rewrite handler name
// 			if nameFunc != nil {
// 				mn = nameFunc(mn)
// 			}
// 			var msgType message.Type
// 			if mt.NumOut() == 0 {
// 				msgType = message.Notify
// 			} else {
// 				msgType = message.Request
// 			}
// 			handler := &Handler{
// 				Method:      method,
// 				IsRawArg:    raw,
// 				MessageType: msgType,
// 			}
// 			if mt.NumIn() == 3 {
// 				handler.Type = mt.In(2)
// 			}
// 			methods[mn] = handler
// 		}
// 	}
// 	return methods
// }
