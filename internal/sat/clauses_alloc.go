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
// a capacity between 2^(i+1) and 2^(i+2)-1 inclusive. The last pool k contains
// slices with a capacity of at least 2^(k+1).
//
// The pools are initiated by the init function below.
var pools = [nPools]sync.Pool{}

func init() {
	for i := 0; i < nPools; i++ {
		capa := 1 << (i + 1)
		pools[i].New = func() any {
			s := make([]Literal, 0, capa)
			return &s
		}
	}
}

// pid returns the ID of the pool responsible for slice of the given capacity.
func pid(capa int) int {
	if capa >= lastCapa {
		return nPools - 1
	}
	pid := bits.Len(uint(capa)) - 1
	if capa < (1 << pid) {
		pid--
	}
	return pid
}

// allocLiteral returns an empty slice that has at least the requested capacity.
func allocSlice(capa int) *[]Literal {
	ref := pools[pid(capa)].Get().(*[]Literal)
	if capa < lastCapa {
		return ref
	}

	// If the slice comes from the last pool, we need to ensure that it has
	// enough capacity. If not, we drop the returned slice and replace it with
	// a newly allocated slice that has the requested capacity.
	if cap(*ref) < capa {
		s := make([]Literal, 0, capa)
		ref = &s
	}

	return ref
}

// freeSlice returns the reference slice so that it can be allocated to another
// clause via allocSlice.
func freeSlice(s *[]Literal) {
	*s = (*s)[:0] // reset the size
	pools[pid(cap(*s))].Put(s)
}
