package sat

import (
	"fmt"
	"strings"
)

type Queue[T any] struct {
	ring  []T
	mask  int
	start int
	end   int
	size  int
}

// New returns a new Queue with the given capacity. Note that the capacity is
// used as an indication and the queue might be instiated with a larger capacity
// than the given one.
func NewQueue[T any](capa int) *Queue[T] {
	capa = nextPower2(capa)
	return &Queue[T]{
		ring:  make([]T, capa),
		mask:  capa - 1,
		start: 0,
		end:   0,
		size:  0,
	}
}

func nextPower2(i int) int {
	i |= i >> 1
	i |= i >> 2
	i |= i >> 4
	i |= i >> 8
	i |= i >> 16
	i |= i >> 32
	return i + 1
}

func (q *Queue[T]) IsEmpty() bool {
	return q.size == 0
}

func (q *Queue[T]) Size() int {
	return q.size
}

func (q *Queue[T]) Clear() {
	q.start = 0
	q.end = 0
	q.size = 0
}

func (q *Queue[T]) Push(elem T) {
	if q.size == len(q.ring) {
		q.resize()
	}
	q.ring[q.end] = elem
	q.end = (q.end + 1) & q.mask
	q.size++
}

func (q *Queue[T]) resize() {
	newRing := make([]T, len(q.ring)*2)
	if q.start == 0 {
		copy(newRing, q.ring)
		q.ring = newRing
		q.mask = len(newRing) - 1
		q.end = q.size
	} else {
		l := len(q.ring) - q.start
		copy(newRing[:l], q.ring[q.start:])
		copy(newRing[l:], q.ring[:q.end])
		q.start = 0
		q.end = len(q.ring)
		q.ring = newRing
		q.mask = len(newRing) - 1
	}
}

func (q *Queue[T]) Pop() T {
	if q.size == 0 {
		panic("pop on an empty queue")
	}
	elem := q.ring[q.start]
	q.start = (q.start + 1) & q.mask
	q.size--
	return elem
}

func (q *Queue[T]) String() string {
	if q.IsEmpty() {
		return "Queue[]"
	}
	sb := strings.Builder{}
	sb.WriteString("Queue[")
	sb.WriteString(fmt.Sprintf("%v", q.ring[q.start]))
	for i := 1; i < q.Size(); i++ {
		p := (q.start + i) & q.mask
		sb.WriteString(fmt.Sprintf(" %v", q.ring[p]))
	}
	sb.WriteByte(']')
	return sb.String()
}
