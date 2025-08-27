package main

import (
	"game/cluster"
	"log"
	"os"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	s := cluster.Server{ServerAddr: os.Args[1], Options: cluster.Options{
		AdvertiseAddr: os.Args[2],
		Name:          os.Args[3],
		ClientAddr:    os.Args[4],
		Frontend:      os.Args[4] != ""},
	}
	if err := s.Startup(); err != nil {
		log.Fatal(err)
		return
	}

	s.Shutdown()
}
