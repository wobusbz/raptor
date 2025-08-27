package component

import (
	"fmt"
	"game/internal/protos"
	"game/session"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/protobuf/proto"
)

type testSession struct {
}

func (t *testSession) ID() int64                            { return 0 }
func (t *testSession) UID() int64                           { return 0 }
func (t *testSession) Push(message proto.Message) error     { return nil }
func (t *testSession) RPC(message proto.Message) error      { return nil }
func (t *testSession) Response(message proto.Message) error { return nil }

type User struct {
}

func (u *User) C2SLogin(aaa session.Session, pb *protos.DelMembersRequest) {
}

func (u *User) Init() {
}

func (u *User) Shutdown() {
}

func (u *User) OnSessionDisconnect(session session.Session) {
}

func (u *User) OnSessionConnect(session session.Session) {
}

func TestIsHandlerMethod(t *testing.T) {
	s := NewService(&User{}, nil)
	s.ExtractHandler()
	args := []reflect.Value{s.Receiver, reflect.ValueOf(&testSession{}), reflect.ValueOf(&clusterpb.CloseSessionRequest{SessionId: 11111111})}
	t.Log(s.Handlers["C2SLogin"].Method.Func.Call(args))

	var ss = map[string]int{"a": 1, "b": 2}
	var n atomic.Int32
	wg := sync.WaitGroup{}
	for i := range 10 {
		wg.Go(func() {
			for range 100000 {
				t.Log("GO ", i, " ", ss["a"])
				n.Add(1)
			}
		})
	}
	wg.Wait()
	fmt.Println(n.Load())
}
