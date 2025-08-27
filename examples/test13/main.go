package main

import (
	"fmt"
	"math/rand"
	"time"
)

func main() {
	fmt.Println(rand.Int() % 5)
	fmt.Println(int64(time.Second.Abs()))
}
