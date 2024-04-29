package sat

import "fmt"

// Literal represents a literal, which either represent a boolean variable or
// its negation.
type Literal int

// VarID returns the ID of the literal's variable.
func (l Literal) VarID() int {
	return int(l) / 2
}

// IsPositive returns true if and only if the literal represent the value of
// its boolean variable (i.e. not its negation)
func (l Literal) IsPositive() bool {
	return l&1 == 0
}

// Opposite returns the opposite literal.
func (l Literal) Opposite() Literal {
	return l ^ 1
}

func (l Literal) String() string {
	if l.IsPositive() {
		return fmt.Sprintf("%d", l.VarID())
	} else {
		return fmt.Sprintf("!%d", l.VarID())
	}
}
