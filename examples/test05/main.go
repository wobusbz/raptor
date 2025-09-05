package main

import (
	"game/server"
	"log"
	"os"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	s := &server.Server{
		Options: server.Options{
			Name:          os.Args[1],
			AdvertiseAddr: os.Args[2],
			ClientAddr:    os.Args[4],
		},
		ServerAddr: os.Args[3],
	}
	s.Startup()
	s.Shutdown()
}
