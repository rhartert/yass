package sat

import (
	"math/bits"
	"sync"
)

// Number of slice pools.
const nPools = 4

// The minimum capacity for slices in the last pool.
const lastCapa = 1 << nPools

// Pools of slices with different capacities so that pool i contains slices with
// a capacity between 2^(i+1) and 2^(i+2)-1 inclusive. The last pool k has no
// upper bound and contains slices with a capacity of at least 2^(k+1).
var pools = [nPools]sync.Pool{}

// pid returns the ID of the smallest pool that can serve a slice of the
// requested capacity.
func pid(capa int) int {
	if lastCapa <= capa {
		return nPools - 1
	}
	pid := max(bits.Len(uint(capa))-1, 0)
	if capa < (1 << pid) {
		pid--
	}
	return pid
}

// allocLiteral returns an empty slice that has at least the requested capacity.
func allocSlice(capa int) *[]Literal {
	pid := pid(capa)

	ref := pools[pid].Get()
	if ref != nil && capa <= cap(*ref.(*[]Literal)) {
		return ref.(*[]Literal)
	}

	if pid < nPools-1 {
		s := make([]Literal, 0, 2<<pid)
		return &s
	}

	if capa <= lastCapa*2 {
		s := make([]Literal, 0, lastCapa*2)
		return &s
	}

	s := make([]Literal, 0, capa)
	return &s
}

// freeSlice returns the reference slice so that it can be allocated to another
// clause via allocSlice.
func freeSlice(s *[]Literal) {
	*s = (*s)[:0] // reset the size
	pools[pid(cap(*s))].Put(s)
}
