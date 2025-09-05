package main

import (
	benchmark "game/benchmark/io"
	"game/examples/test14/protos"
	"log"
	_ "net/http/pprof"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	conn := benchmark.NewConnector()
	if err := conn.Start("127.0.0.1:8080"); err != nil {
		log.Fatal(err)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			log.Println(conn.Request(&protos.C2SLogin{Name: "hello world"}))
		}
	}
}
