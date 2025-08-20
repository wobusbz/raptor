package main

import (
	"context"
	"game/internal/protos"
	"log"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	conn, err := grpc.NewClient("127.0.0.1:8800", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Debug("[Test08] Dial failed", slog.Any("tcp", err))
		return
	}
	pbconn := protos.NewRemoteServerClient(conn)

	ticker := time.NewTicker(time.Second)

	for c := range ticker.C {
		_, err = pbconn.Call(context.TODO(), &protos.CallRequest{Data: []byte("hello world")})
		if err != nil {
			log.Println(err)
		}
		log.Println(c.Unix())
	}
}
