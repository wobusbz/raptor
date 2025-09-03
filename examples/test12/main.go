package main

import (
	"game/benchmark/io"
	"game/internal/protos"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"sync/atomic"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:8999", nil))
	}()
	var n atomic.Int32
	for range 3000 * 2 {
		connector := io.NewConnector()
		connector.OnConnected(func(data []byte) {
		})
		connector.Start("127.0.0.1:8080")
		ticker := time.NewTicker(time.Second * time.Duration(rand.Int()%5+1))
		defer ticker.Stop()
		go func() {
			for range ticker.C {
				if err := connector.Request("CENT/USER/C2SLogin", &protos.DelMembersRequest{ServiceName: "Hello world", InstanceId: "111111111"}, func(data any) {
				}); err != nil {
					log.Println(err)
					return
				}
			}
		}()
		n.Add(1)
		log.Println(n.Load())
	}
	log.Println(n.Load())
	select {}
}
