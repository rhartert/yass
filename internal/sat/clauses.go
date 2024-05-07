package sat

import (
	"strings"
)

type Clause struct {
	learnt   bool
	activity float64

	// The clause's literals. Must always contain at least two literals.
	literals []Literal
}

func NewClause(s *Solver, literals []Literal, learnt bool) (*Clause, bool) {

	if !learnt {
		size := len(literals)
		seen := map[Literal]struct{}{}

		for i := size - 1; i >= 0; i-- {
			// If the opposite literal is in the clause, then the clause is
			// always true.
			if _, ok := seen[literals[i].Opposite()]; ok {
				return nil, true
			}

			// Remove the literal if it is already present.
			if _, ok := seen[literals[i]]; ok {
				size--
				literals[i], literals[size] = literals[size], literals[i]
			}

			seen[literals[i]] = struct{}{}

			switch s.LitValue(literals[i]) {
			case True:
				return nil, true // clause is always true
			case False:
				size--
				literals[i], literals[size] = literals[size], literals[i]
			}
		}

		literals = literals[:size]
	}

	switch len(literals) {
	case 0:
		// Empty clauses cannot be valid.
		return nil, false
	case 1:
		// Directly enqueue unit facts.
		return nil, s.enqueue(literals[0], nil)
	default:
		// Actually create the clause.
		c := &Clause{}
		c.literals = literals
		c.learnt = learnt

		if learnt {
			maxLevel := -1
			wl := -1
			for i := 1; i < len(c.literals); i++ {
				if level := s.level[c.literals[i].VarID()]; level > maxLevel {
					maxLevel = level
					wl = i
				}
			}
			c.literals[wl], c.literals[1] = c.literals[1], c.literals[wl]

			// Bumping.
			s.BumpClaActivity(c)
			for _, l := range c.literals {
				s.BumpVarActivity(l)
			}
		}

		s.Watch(c, c.literals[0].Opposite())
		s.Watch(c, c.literals[1].Opposite())

		return c, true
	}
}

func (c *Clause) locked(solver *Solver) bool {
	return solver.reason[c.literals[0].VarID()] == c
}

func (c *Clause) Remove(s *Solver) {
	s.Unwatch(c, c.literals[0].Opposite())
	s.Unwatch(c, c.literals[1].Opposite())
}

func (c *Clause) Simplify(s *Solver) bool {
	j := 0
	for i := 0; i < len(c.literals); i++ {
		v := s.LitValue(c.literals[i])
		switch v {
		case True:
			return true
		case False:
			// discard the literal.
		case Unknown:
			c.literals[j] = c.literals[i]
			j++
		}
	}
	c.literals = c.literals[:j]
	return false
}

func (c *Clause) Propagate(s *Solver, l Literal) bool {
	// Make sure the false literal is c.literals[1].
	opp := l.Opposite()
	if c.literals[0] == opp {
		c.literals[0] = c.literals[1]
		c.literals[1] = opp
	}

	// If c.literals[0] is True, then the clause is already true.
	if s.LitValue(c.literals[0]) == True {
		s.Watch(c, l)
		return true
	}

	// Look for a new literal to watch. If another literal set to true is found,
	// then the clause is already true.
	for i := 2; i < len(c.literals); i++ {
		if s.LitValue(c.literals[i]) != False {
			c.literals[1] = c.literals[i]
			c.literals[i] = l.Opposite()
			s.Watch(c, c.literals[1].Opposite())
			return true
		}
	}

	// The first literal must be true if all other literals are false.
	s.Watch(c, l)
	return s.enqueue(c.literals[0], c)
}

func (c *Clause) ExplainFailure(s *Solver) []Literal {
	s.tmpReason = s.tmpReason[:0]
	for _, l := range c.literals {
		s.tmpReason = append(s.tmpReason, l.Opposite())
	}
	if c.learnt {
		s.BumpClaActivity(c)
	}
	return s.tmpReason
}

func (c *Clause) ExplainAssign(s *Solver, l Literal) []Literal {
	s.tmpReason = s.tmpReason[:0]
	for i := 1; i < len(c.literals); i++ {
		s.tmpReason = append(s.tmpReason, c.literals[i].Opposite())
	}
	if c.learnt {
		s.BumpClaActivity(c)
	}
	return s.tmpReason
}

func (c *Clause) String() string {
	if len(c.literals) == 0 {
		return "Clause[]"
	}
	sb := strings.Builder{}
	sb.WriteString("Clause[")
	sb.WriteString(c.literals[0].String())
	for _, l := range c.literals[1:] {
		sb.WriteByte(' ')
		sb.WriteString(l.String())
	}
	sb.WriteByte(']')
	return sb.String()
}
