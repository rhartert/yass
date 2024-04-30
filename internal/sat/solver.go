package sat

import (
	"fmt"
	"log"
	"sort"
	"time"
)

type Solver struct {
	// Clause database.
	constraints []Constraint
	learnts     []*Clause
	clauseInc   float64
	clauseDecay float64

	// Variable ordering.
	activities []float64
	varInc     float64
	varDecay   float64
	order      *VarOrder

	// Propagation and watchers.
	watchers  [][]Constraint
	propQueue *Queue[Literal]

	// Assignments.
	assigns  []LBool
	trail    []Literal
	trailLim []int
	reason   []Constraint
	level    []int

	// Whether the problem has reached a top level conflict.
	unsat bool

	// Search statistics.
	TotalConflicts  int64
	TotalRestarts   int64
	TotalIterations int64
	startTime       time.Time

	// Stop conditions.
	hasStopCond bool
	maxConflict int64

	// Models.
	Models [][]bool

	// Temporary slice used in the Propagate function. The slice is re-used by
	// all Propagate calls to avoid unnecessarily allocating new slices.
	tmpWatchers []Constraint

	// Temporary slice used in Analyze to accumulate literals before these are
	// used to create a new learnt clause. Having one shared buffer between all
	// call reduces the overhead of having to grow each time Analye is called.
	tmpLearnts []Literal

	// Used for clause to explain themselves.
	tmpReason []Literal
}

type Options struct {
	ClauseDecay   float64
	VariableDecay float64

	MaxConflicts int64
}

var DefaultOptions = Options{
	ClauseDecay:   0.999,
	VariableDecay: 0.95,
	MaxConflicts:  -1,
}

// NewDefaultSolver returns a solver configured with default options. This is
// equivalent to calling NewSolver with DefaultOptions.
func NewDefaultSolver() *Solver {
	return NewSolver(DefaultOptions)
}

func NewSolver(ops Options) *Solver {
	s := &Solver{
		clauseDecay: ops.ClauseDecay,
		varDecay:    ops.VariableDecay,
		clauseInc:   0.1,
		varInc:      0.1,
		propQueue:   NewQueue[Literal](128),
	}

	if ops.MaxConflicts >= 0 {
		s.hasStopCond = true
		s.maxConflict = ops.MaxConflicts
	}

	return s
}

func (s *Solver) shouldStop() bool {
	if !s.hasStopCond {
		return false
	}
	if s.maxConflict >= 0 && s.maxConflict <= s.TotalConflicts {
		return true
	}

	return false
}

func (s *Solver) PositiveLiteral(varID int) Literal {
	return Literal(varID * 2)
}

func (s *Solver) NegativeLiteral(varID int) Literal {
	return s.PositiveLiteral(varID).Opposite()
}

func (s *Solver) NumVariables() int {
	return len(s.assigns)
}

func (s *Solver) NumAssigns() int {
	return len(s.trail)
}

func (s *Solver) NumConstraints() int {
	return len(s.constraints)
}

func (s *Solver) NumLearnts() int {
	return len(s.learnts)
}

func (s *Solver) VarValue(x int) LBool {
	return s.assigns[x]
}

func (s *Solver) LitValue(l Literal) LBool {
	lb := s.assigns[l.VarID()]
	if l.IsPositive() {
		return lb
	} else {
		return lb.Opposite()
	}
}

func (s *Solver) AddVariable() int {
	index := s.NumVariables()
	s.watchers = append(s.watchers, nil)
	s.watchers = append(s.watchers, nil)
	s.reason = append(s.reason, nil)
	s.assigns = append(s.assigns, Unknown)
	s.level = append(s.level, -1)
	s.activities = append(s.activities, 0)
	s.order.NewVar()
	return index
}

func (s *Solver) AddClause(clause []Literal) error {
	if s.decisionLevel() != 0 {
		return fmt.Errorf("can only add clauses at the root level")
	}
	c, ok := NewClause(s, clause, false)
	if c != nil {
		s.constraints = append(s.constraints, c)
	}
	if !ok {
		s.unsat = true
	}

	return nil
}

func (s *Solver) Simplify() bool {
	if s.Propagate() != nil {
		return false
	}

	if s.propQueue.Size() != 0 {
		log.Fatal("propQueue should be empty when calling simplify")
	}

	j := 0
	for i := 0; i < len(s.learnts); i++ {
		if s.learnts[i].Simplify(s) {
			s.learnts[i].Remove(s)
		} else {
			s.learnts[j] = s.learnts[i]
			j++
		}
	}
	s.learnts = s.learnts[:j]

	j = 0
	for i := 0; i < len(s.constraints); i++ {
		if s.constraints[i].Simplify(s) {
			s.constraints[i].Remove(s)
		} else {
			s.constraints[j] = s.constraints[i]
			j++
		}
	}
	s.constraints = s.constraints[:j]

	return true
}

func (s *Solver) ReduceDB() {
	lim := s.clauseInc / float64(len(s.learnts))

	sort.Slice(s.learnts, func(i, j int) bool {
		return s.learnts[i].activity < s.learnts[j].activity
	})

	i, j := 0, 0
	for ; i < len(s.learnts)/2; i++ {
		if s.learnts[i].locked(s) {
			s.learnts[j] = s.learnts[i]
			j++
		} else {
			s.learnts[i].Remove(s)
		}
	}

	for ; i < len(s.learnts); i++ {
		if !s.learnts[i].locked(s) && s.learnts[i].activity < lim {
			s.learnts[i].Remove(s)
		} else {
			s.learnts[j] = s.learnts[i]
			j++
		}
	}

	s.learnts = s.learnts[:j]
}

func (s *Solver) decisionLevel() int {
	return len(s.trailLim)
}

func (s *Solver) Solve() LBool {
	numConflicts := 100
	numLearnts := s.NumConstraints() / 3
	status := Unknown

	s.startTime = time.Now()
	s.order = NewVarOrder(s, s.NumVariables())

	s.printSeparator()
	s.printSearchHeader()
	s.printSeparator()

	for status == Unknown {
		status = s.Search(numConflicts, numLearnts)
		numConflicts += numConflicts / 10
		numLearnts += numLearnts / 20

		if s.shouldStop() {
			break
		}
	}

	s.printSearchStats()
	s.printSeparator()

	s.cancelUntil(0)
	return status
}

func (s *Solver) BumpClaActivity(c *Clause) {
	c.activity += s.clauseInc
	if c.activity > 1e100 {
		for _, l := range s.learnts {
			l.activity = l.activity / 1e100
		}
	}
}

func (s *Solver) BumpVarActivity(l Literal) {
	vid := l.VarID()
	s.activities[vid] += s.varInc
	if s.activities[vid] > 1e100 {
		for i, a := range s.activities {
			s.activities[i] = a / 1e100
		}
	}
	s.order.Update(vid)
}

func (s *Solver) DecayClaActivity() {
	s.clauseInc *= s.clauseDecay
}

func (s *Solver) DecayVarActivity() {
	s.varInc *= s.varDecay
}

func (s *Solver) DecayActivities() {
	s.DecayClaActivity()
	s.DecayVarActivity()
}

func (s *Solver) Propagate() Constraint {
	for s.propQueue.Size() > 0 {
		l := s.propQueue.Pop()

		s.tmpWatchers = s.tmpWatchers[:0]
		s.tmpWatchers = append(s.tmpWatchers, s.watchers[l]...)
		s.watchers[l] = s.watchers[l][:0]

		// Detach the constraint from the literal.
		for i, c := range s.tmpWatchers {
			if c.Propagate(s, l) {
				continue
			}

			// Constraint is conflicting, copy remaining watchers
			// and return the constraint.
			s.watchers[l] = append(s.watchers[l], s.tmpWatchers[i+1:]...)
			s.propQueue.Clear()
			return s.tmpWatchers[i]
		}
	}

	return nil
}

func (s *Solver) enqueue(l Literal, from Constraint) bool {
	switch v := s.LitValue(l); v {
	case False:
		return false // conflicting assignment
	case True:
		return true // already assigned
	default:
		// New fact, store it.
		varID := l.VarID()
		s.assigns[varID] = Lift(l.IsPositive())
		s.level[varID] = s.decisionLevel()
		s.reason[varID] = from
		s.trail = append(s.trail, l)
		s.propQueue.Push(l)
		return true
	}
}

func (s *Solver) explain(c Constraint, l Literal) []Literal {
	if l == -1 {
		return c.ExplainFailure(s)
	} else {
		return c.ExplainAssign(s, l)
	}
}

func (s *Solver) analyze(confl Constraint) ([]Literal, int) {
	l := Literal(-1) // unknown literal
	seen := make([]bool, len(s.assigns))
	counter := 0
	backtrackLevel := 0

	// Note that the first element is already reserved.
	s.tmpLearnts = s.tmpLearnts[:0]

	for {
		// Trace reason.
		for _, q := range s.explain(confl, l) {
			v := q.VarID()
			if seen[v] {
				continue
			}

			seen[v] = true
			if s.level[v] == s.decisionLevel() {
				counter++
				continue
			}

			s.tmpLearnts = append(s.tmpLearnts, q.Opposite())
			if level := s.level[v]; level > backtrackLevel {
				backtrackLevel = level
			}
		}

		// Select next literal to look at.
		for {
			l = s.trail[len(s.trail)-1]
			confl = s.reason[l.VarID()]
			s.undoOne()
			if seen[l.VarID()] {
				break
			}
		}

		counter--
		if counter <= 0 {
			break
		}
	}

	learnts := make([]Literal, len(s.tmpLearnts)+1)
	learnts[0] = l.Opposite()
	copy(learnts[1:], s.tmpLearnts)

	return learnts, backtrackLevel
}

func (s *Solver) record(clause []Literal) {
	c, _ := NewClause(s, clause, true)
	s.enqueue(clause[0], c)
	if c != nil {
		s.learnts = append(s.learnts, c)
	}
}

func (s *Solver) Search(nConflicts int, nLearnts int) LBool {
	if s.unsat {
		return False
	}

	s.TotalRestarts++
	conflictCount := 0

	for !s.shouldStop() {
		if s.TotalIterations%10000 == 0 {
			s.printSearchStats()
		}
		s.TotalIterations++

		if conflict := s.Propagate(); conflict != nil {
			conflictCount++
			s.TotalConflicts++

			if s.decisionLevel() == 0 {
				s.unsat = true
				return False
			}

			learntClause, backtrackLevel := s.analyze(conflict)
			if backtrackLevel > 0 {
				s.cancelUntil(backtrackLevel)
			} else {
				s.cancelUntil(0)
			}

			s.record(learntClause)
			s.DecayActivities()
			continue
		}

		// No Conflict
		// -----------

		if s.decisionLevel() == 0 {
			s.Simplify()
		}

		if len(s.learnts)-s.NumAssigns() >= nLearnts {
			s.ReduceDB()
		}

		if s.NumAssigns() == s.NumVariables() { // solution found
			s.saveModel()
			s.cancelUntil(0)
			return True
		}

		if conflictCount > nConflicts {
			s.cancelUntil(0)
			return Unknown
		}

		l := s.order.Select()
		s.assume(l)
	}

	return Unknown
}

func (s *Solver) undoOne() {
	l := s.trail[len(s.trail)-1]
	v := l.VarID()
	s.assigns[v] = Unknown
	s.reason[v] = nil
	s.level[v] = -1
	s.order.Undo(v)
	s.trail = s.trail[:len(s.trail)-1]
}

func (s *Solver) assume(l Literal) bool {
	s.trailLim = append(s.trailLim, len(s.trail))
	return s.enqueue(l, nil)
}

func (s *Solver) cancel() {
	c := len(s.trail) - s.trailLim[len(s.trailLim)-1]
	for ; c != 0; c-- {
		s.undoOne()
	}
	s.trailLim = s.trailLim[:len(s.trailLim)-1]
}

func (s *Solver) cancelUntil(level int) {
	for s.decisionLevel() > level {
		s.cancel()
	}
}

func (s *Solver) saveModel() {
	model := make([]bool, len(s.assigns))
	for i, lb := range s.assigns {
		if lb == Unknown {
			panic("not a model")
		}
		model[i] = lb == True
	}
	s.Models = append(s.Models, model)
}

func (s *Solver) printSeparator() {
	fmt.Println("c ---------------------------------------------------------------------------")
}

func (s *Solver) printSearchHeader() {
	fmt.Println("c            time     iterations      conflicts       restarts        learnts")
}

func (s *Solver) printSearchStats() {
	fmt.Printf(
		"c %14.3fs %14d %14d %14d %14d\n",
		time.Since(s.startTime).Seconds(),
		s.TotalIterations,
		s.TotalConflicts,
		s.TotalRestarts,
		len(s.learnts))
}
