package main

import (
	"game/cluster"
	"log"
	"os"
)

func main() {
	s := cluster.Server{ServerAddr: os.Args[1], Options: cluster.Options{AdvertiseAddr: os.Args[2], Name: os.Args[3]}}
	if err := s.Startup(); err != nil {
		log.Fatal(err)
		return
	}
	s.Shutdown()
}
