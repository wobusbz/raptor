package main

import (
	"game/cluster"
	"log"
	"os"
)

func main() {
	s := cluster.Server{ServerAddr: os.Args[1], Options: cluster.Options{IsMaster: true, Name: os.Args[2]}}
	if err := s.Startup(); err != nil {
		log.Fatal(err)
		return
	}
	s.Shutdown()
}
