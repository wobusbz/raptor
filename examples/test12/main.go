package main

import (
	benchmark "game/benchmark/io"
	"game/examples/test14/protos"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	for range 10000 * 2 {
		time.Sleep(5 * time.Millisecond)
		go func() {
			conn := benchmark.NewConnector()
			if err := conn.Start("127.0.0.1:8080"); err != nil {
				log.Println(err)
				return
			}
			n := (rand.Int() % 5) + 1
			ticker := time.NewTicker(time.Duration(n) * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				_ = conn.Request(&protos.C2SLogin{Name: "hello world"})
			}
		}()
	}

	select {}
}
