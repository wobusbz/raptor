package queue

import (
	"testing"
)

func BenchmarkQueue_PushSingleConsumer(b *testing.B) {
	q := New[int]()
	var stop int32
	done := make(chan struct{})
	go func() {
		for i := 0; i < b.N; {
			if _, ok := q.Pop(); ok {
				i++
			}
		}
		close(done)
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(i)
	}
	<-done
	_ = stop
}

func BenchmarkBasicQueue(b *testing.B) {
	q := New[int]()
	done := make(chan struct{})
	go func() {
		for i := 0; i < b.N; {
			if _, ok := q.Pop(); ok {
				i++
			}
		}
		close(done)
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(i)
	}
	<-done
}
