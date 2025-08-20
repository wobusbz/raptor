package queue

import (
	"sync/atomic"
)

type Queue[T any] struct {
	head atomic.Pointer[node[T]]
	tail *node[T]
}

type node[T any] struct {
	next    atomic.Pointer[node[T]]
	element T
}

func New[T any]() *Queue[T] {
	stub := &node[T]{}
	q := &Queue[T]{tail: stub}
	q.head.Store(stub)
	return q
}

func (q *Queue[T]) Push(v T) {
	n := &node[T]{element: v}
	prev := q.head.Swap(n)
	prev.next.Store(n)
}

func (q *Queue[T]) Pop() (T, bool) {
	t := q.tail
	n := t.next.Load()
	var zero T
	if n == nil {
		return zero, false
	}
	q.tail = n
	el := n.element
	n.element = zero
	return el, true
}

func (q *Queue[T]) Empty() bool {
	return q.tail.next.Load() == nil
}
