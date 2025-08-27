package main

import (
	"game/cluster"
	"game/internal/protos"
	"game/session"
	"log"
	"os"
)

type User struct {
}

func (u *User) C2SLogin(aaa session.Session, pb *protos.DelMembersRequest) {
	aaa.Push(&protos.NewMembersRequest{Instances: &protos.ServiceInstance{
		InstanceId:  "11111111111111",
		ServiceName: "Hello world",
		Addr:        "127.0.0.1:9999",
	}})
}

func (u *User) Init() {
}

func (u *User) Shutdown() {
}

func (u *User) OnSessionDisconnect(session session.Session) {
}

func (u *User) OnSessionConnect(session session.Session) {
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	s := cluster.Server{ServerAddr: os.Args[1], Options: cluster.Options{IsMaster: true, Name: os.Args[2]}}
	if err := s.Startup(); err != nil {
		log.Fatal(err)
		return
	}
	s.Register(&User{})
	s.Shutdown()
}
