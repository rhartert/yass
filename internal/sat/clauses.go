package sat

import (
	"strings"
)

type status uint8

const (
	statusDeleted   status = 0b001
	statusLearnt    status = 0b010
	statusProtected status = 0b100
)

type Clause struct {
	activity float64

	// The clause's literals. The slice contains at least two literals if the
	// clause is active, it is nil if the clause has been marked as deleted.
	literals []Literal

	// If true, the clause will not be deleted in the next clause DB clean up.
	// This is only relevant to learnt clauses.
	statusMask status

	// The literal block distance used to estimate the quality of the clause.
	lbd int
}

func (c *Clause) isProtected() bool {
	return c.statusMask&statusProtected != 0
}

func (c *Clause) setProtected() {
	c.statusMask |= statusProtected
}

func (c *Clause) setUnprotected() {
	c.statusMask &= ^statusProtected
}

func (c *Clause) isLearnt() bool {
	return c.statusMask&statusLearnt != 0
}

func NewClause(s *Solver, tmpLiterals []Literal, learnt bool) (*Clause, bool) {
	size := len(tmpLiterals)

	if !learnt {
		seen := map[Literal]struct{}{}

		for i := size - 1; i >= 0; i-- {
			// If the opposite literal is in the clause, then the clause is
			// always true.
			if _, ok := seen[tmpLiterals[i].Opposite()]; ok {
				return nil, true
			}

			// Remove the literal if it is already present.
			if _, ok := seen[tmpLiterals[i]]; ok {
				size--
				tmpLiterals[i], tmpLiterals[size] = tmpLiterals[size], tmpLiterals[i]
			}

			seen[tmpLiterals[i]] = struct{}{}

			switch s.LitValue(tmpLiterals[i]) {
			case True:
				return nil, true // clause is always true
			case False:
				size--
				tmpLiterals[i], tmpLiterals[size] = tmpLiterals[size], tmpLiterals[i]
			}
		}

		tmpLiterals = tmpLiterals[:size]
	}

	switch size {
	case 0:
		// Empty clauses cannot be valid.
		return nil, false
	case 1:
		// Directly enqueue unit facts.
		return nil, s.enqueue(tmpLiterals[0], nil)
	default:
		// Actually create the clause.
		c := &Clause{}

		c.literals = make([]Literal, size)
		copy(c.literals, tmpLiterals)

		if learnt {
			c.statusMask |= statusLearnt

			maxLevel := -1
			wl := -1
			for i := 1; i < len(c.literals); i++ {
				if level := s.assignLevels[c.literals[i].VarID()]; level > maxLevel {
					maxLevel = level
					wl = i
				}
			}
			c.literals[wl], c.literals[1] = c.literals[1], c.literals[wl]
		}

		s.Watch(c, c.literals[0].Opposite(), c.literals[1])
		s.Watch(c, c.literals[1].Opposite(), c.literals[0])

		return c, true
	}
}

func (c *Clause) locked(solver *Solver) bool {
	return solver.assignReasons[c.literals[0].VarID()] == c
}

func (c *Clause) Delete(s *Solver) {
	c.statusMask |= statusDeleted

	s.Unwatch(c, c.literals[0].Opposite())
	s.Unwatch(c, c.literals[1].Opposite())

	// Cut the reference to the slice of literals so that it can be garbage
	// collected even if the clause itself is still referenced.
	c.literals = nil
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
	// Make sure that the triggering literal is c.literals[1]. This simplifies
	// the rest of this function as c.literals[0] is always the literal to be
	// potentially enqueued (if all other literals are false).
	opp := l.Opposite()
	if c.literals[0] == opp {
		c.literals[0] = c.literals[1]
		c.literals[1] = opp
	}

	// If c.literals[0] is True, then the clause is already true.
	if s.LitValue(c.literals[0]) == True {
		s.Watch(c, l, c.literals[0])
		return true
	}

	// Look for a new literal to watch. If another literal set to true is found,
	// then the clause is already true.
	for i := 2; i < len(c.literals); i++ {
		if s.LitValue(c.literals[i]) != False {
			c.literals[1] = c.literals[i]
			c.literals[i] = l.Opposite()
			s.Watch(c, c.literals[1].Opposite(), c.literals[0])
			return true
		}
	}

	// The first literal must be true if all other literals are false.
	s.Watch(c, l, c.literals[0])
	return s.enqueue(c.literals[0], c)
}

func (c *Clause) explainConflict(outReason *[]Literal) {
	exp := (*outReason)[:0]
	for _, l := range c.literals {
		exp = append(exp, l.Opposite())
	}
	*outReason = exp
}

func (c *Clause) explainAssign(outReason *[]Literal) {
	exp := (*outReason)[:0]
	for _, l := range c.literals[1:] {
		exp = append(exp, l.Opposite())
	}
	*outReason = exp
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
