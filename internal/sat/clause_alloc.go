//go:build !clausepool

package sat

func newClause(literals []Literal, learnt bool) *Clause {
	c := &Clause{}
	c.learnt = learnt
	c.literals = make([]Literal, 0, len(literals))
	c.literals = append(c.literals, literals...)
	return c
}

func freeClause(c *Clause) {}
