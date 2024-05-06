//go:build clausepool

package sat

import "sync"

var pool8 = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		s := make([]Literal, 0, 8)
		return &s
	},
}

var pool64 = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		s := make([]Literal, 0, 64)
		return &s
	},
}

var pool256 = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		s := make([]Literal, 0, 256)
		return &s
	},
}

var poolHuge = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		s := make([]Literal, 0, 512)
		return &s
	},
}

func newClause(literals []Literal, learnt bool) *Clause {
	c := &Clause{}
	c.learnt = learnt

	switch l := len(literals); {
	case l <= 8:
		c.sliceRef = pool8.Get().(*[]Literal)
	case l <= 64:
		c.sliceRef = pool64.Get().(*[]Literal)
	case l <= 256:
		c.sliceRef = pool256.Get().(*[]Literal)
	default:
		c.sliceRef = poolHuge.Get().(*[]Literal)
	}

	// Get a base slice from the slice pool.
	c.literals = *c.sliceRef
	c.literals = c.literals[0:0] // reset
	c.literals = append(c.literals, literals...)

	return c
}

func freeClause(c *Clause) {
	*c.sliceRef = c.literals

	switch l := len(c.literals); {
	case l >= 512:
		poolHuge.Put(c.sliceRef)
	case l >= 256:
		pool256.Put(c.sliceRef)
	case l >= 64:
		pool64.Put(c.sliceRef)
	default:
		pool8.Put(c.sliceRef)
	}
}
