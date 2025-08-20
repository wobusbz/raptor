package main

import (
	"crypto/rand"
	"fmt"
)

func SimpleUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x", b[0:2], b[2:4], b[4:6], b[6:])
}

func main() {
	fmt.Println(SimpleUUID())
}
