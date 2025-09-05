package main

import (
	"game/examples/test14/protos"
	"game/server"
	"game/session"
	"log"
	"os"
)

type User struct {
	app *server.Server
}

func (u *User) C2SLogin(session session.Session, pb *protos.C2SLogin) {
	// log.Println(pb)
}

func (u *User) Init() {
	log.Println("初始化User")
}

func (u *User) Shutdown() {
	log.Println("关闭User")
}

func (u *User) OnSessionDisconnect(session session.Session) {
	log.Println("Session 连接关闭: ", session.ID())
}

func (u *User) OnSessionConnect(session session.Session) {
	log.Println("Session 连接成功: ", session.ID())
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	s := &server.Server{Options: server.Options{Name: os.Args[1]}, ServerAddr: os.Args[2]}
	s.Startup()
	s.Register(&User{app: s})
	s.Shutdown()
}
