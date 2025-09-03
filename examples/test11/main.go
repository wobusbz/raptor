package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"
)

type Msg struct {
	Type    string
	Payload any
	Reply   chan any
}

type Actor struct {
	id    string
	inbox chan Msg
}

func (a *Actor) loop() {
	for m := range a.inbox {
		m.Reply <- 1
	}
}

func (a *Actor) Ask(ctx context.Context, m Msg, timeout time.Duration) (any, error) {
	m.Reply = make(chan any, 1)
	select {
	case a.inbox <- m:
	default:
		return nil, errors.New("mailbox full")
	}
	select {
	case r := <-m.Reply:
		return r, nil
	case <-time.After(timeout):
		return nil, context.DeadlineExceeded
	}
}

func main() {
	actor := &Actor{id: "1", inbox: make(chan Msg, 1)}
	go actor.loop()
	log.Println(actor.Ask(context.TODO(), Msg{Type: "1", Payload: "helloworld"}, time.Second))

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for v := range ticker.C {
		hrdata := map[string]any{
			"code": 200,
			"sys": map[string]any{
				"heartbeat":  time.Second,
				"servertime": v.UTC().Unix(),
			},
		}
		hrd, err := json.Marshal(hrdata)
		if err != nil {
			log.Println(err)
			return
		}
		fmt.Println(string(hrd))
	}
}
