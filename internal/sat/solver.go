package sat

import (
	"fmt"
	"log"
	"sort"
	"time"
)

type Solver struct {
	// Clause database.
	constraints []*Clause
	learnts     []*Clause
	clauseInc   float64
	clauseDecay float64

	nextReduce     int64
	nextReduceIncr int64

	// Variable ordering.
	activities []float64
	varInc     float64
	varDecay   float64
	order      *VarOrder

	// Propagation and watchers.
	watchers  [][]*Clause
	propQueue *Queue[Literal]

	// Value assigned ot each literal.
	assigns []LBool

	// Trail.
	trail    []Literal
	trailLim []int
	reason   []*Clause
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
	timeout     time.Duration

	// Models.
	Models [][]bool

	// Temporary slice used in the Propagate function. The slice is re-used by
	// all Propagate calls to avoid unnecessarily allocating new slices.
	tmpWatchers []*Clause

	// Temporary slice used in Analyze to accumulate literals before these are
	// used to create a new learnt clause. Having one shared buffer between all
	// call reduces the overhead of having to grow each time Analye is called.
	tmpLearnts []Literal

	// Used for clause to explain themselves.
	tmpReason []Literal

	// Shared by operation that needs to put variables in a set and empty that
	// set efficiently.
	seenVar *ResetSet

	// Shared by operation that needs to put the decision levels in a set and
	// empty that set efficiently. This could technically be done using seenVar
	// but some operations (e.g. analyze) needs to maintain both set at the same
	// time.
	seenLevel *ResetSet
}

type Options struct {
	ClauseDecay   float64
	VariableDecay float64

	MaxConflicts int64
	Timeout      time.Duration
}

var DefaultOptions = Options{
	ClauseDecay:   0.999,
	VariableDecay: 0.95,
	MaxConflicts:  -1,
	Timeout:       -1,
}

// NewDefaultSolver returns a solver configured with default options. This is
// equivalent to calling NewSolver with DefaultOptions.
func NewDefaultSolver() *Solver {
	return NewSolver(DefaultOptions)
}

func NewSolver(ops Options) *Solver {
	s := &Solver{
		clauseDecay:    ops.ClauseDecay,
		varDecay:       ops.VariableDecay,
		clauseInc:      0.1,
		varInc:         0.1,
		propQueue:      NewQueue[Literal](128),
		maxConflict:    -1,
		timeout:        -1,
		nextReduce:     4000,
		nextReduceIncr: 300,
		seenLevel:      &ResetSet{},
		seenVar:        &ResetSet{},
		tmpLearnts:     make([]Literal, 0, 32),
		tmpReason:      make([]Literal, 0, 32),
	}

	if ops.MaxConflicts >= 0 {
		s.hasStopCond = true
		s.maxConflict = ops.MaxConflicts
	}
	if ops.Timeout >= 0 {
		s.hasStopCond = true
		s.timeout = ops.Timeout
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
	if s.timeout >= 0 && s.timeout <= time.Since(s.startTime) {
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
	return len(s.assigns) / 2
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
	return s.assigns[s.PositiveLiteral(x)]
}

func (s *Solver) LitValue(l Literal) LBool {
	return s.assigns[l]
}

func (s *Solver) AddVariable() int {
	index := s.NumVariables()
	s.watchers = append(s.watchers, nil)
	s.watchers = append(s.watchers, nil)
	s.reason = append(s.reason, nil)

	s.seenVar.Expand()
	s.seenLevel.Expand()

	// One for each literal.
	s.assigns = append(s.assigns, Unknown)
	s.assigns = append(s.assigns, Unknown)

	s.level = append(s.level, -1)
	s.activities = append(s.activities, 0)
	s.order.NewVar()
	return index
}

// Watch registers clause c to be awaken when Literal watch is assigned to true.
func (s *Solver) Watch(c *Clause, watch Literal) {
	s.watchers[watch] = append(s.watchers[watch], c)
}

// Unwatch removes clause c from the list of watchers.
func (s *Solver) Unwatch(c *Clause, watch Literal) {
	j := 0
	for i := 0; i < len(s.watchers[watch]); i++ {
		if s.watchers[watch][i] != c {
			s.watchers[watch][j] = s.watchers[watch][i]
			j++
		}
	}
	s.watchers[watch] = s.watchers[watch][:j]
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

// Simplify simplifies the clause DB as well as the problem clauses according
// to the root-level assignments. Clauses that are satisfied at the root-level
// are removed.
func (s *Solver) Simplify() bool {
	if l := s.decisionLevel(); l != 0 {
		log.Fatalf("Simplify called on non root-level: %d", l)
	}
	if s.propQueue.Size() != 0 {
		log.Fatal("propQueue should be empty when calling simplify")
	}

	if s.unsat || s.Propagate() != nil {
		s.unsat = true
		return false
	}

	s.simplifyPtr(&s.learnts)
	s.simplifyPtr(&s.constraints) // could be turned off

	return true
}

// simplifyPtr simplifies the clauses in the given slice and remove clauses that
// are already satisfied.
func (s *Solver) simplifyPtr(clausesPtr *[]*Clause) {
	clauses := *clausesPtr
	j := 0
	for i := 0; i < len(clauses); i++ {
		if clauses[i].Simplify(s) {
			clauses[i].Remove(s)
		} else {
			clauses[j] = clauses[i]
			j++
		}
	}
	*clausesPtr = clauses[:j]
}

func (s *Solver) ReduceDB() {
	// Sort learnt clauses from "the worst" to "the best".
	sort.Slice(s.learnts, func(i, j int) bool {
		ci := s.learnts[i]
		cj := s.learnts[j]

		switch {
		case len(ci.literals) > 2 && len(cj.literals) == 2:
			return true
		case ci.lbd > cj.lbd:
			return true
		case ci.activity < cj.activity:
			return true
		default:
			return false
		}
	})

	// Protect the 10% best clause.
	for i := len(s.learnts) * 90 / 100; i < len(s.learnts); i++ {
		s.learnts[i].isProtected = true
	}

	toDelete := len(s.learnts) / 2

	i, j := 0, 0
	for ; i < len(s.learnts); i++ {
		c := s.learnts[i]

		if toDelete > 0 && !c.locked(s) && c.lbd > 2 && len(c.literals) > 2 && !c.isProtected {
			toDelete--
			c.Remove(s)
		} else {
			if c.isProtected {
				c.isProtected = false
				toDelete++
			}
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
	status := Unknown
	s.order = NewVarOrder(s, s.NumVariables())
	s.startTime = time.Now()

	s.printSeparator()
	s.printSearchHeader()
	s.printSeparator()

	for status == Unknown {
		status = s.Search(numConflicts)
		numConflicts += 1000

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

func (s *Solver) Propagate() *Clause {
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

func (s *Solver) enqueue(l Literal, from *Clause) bool {
	switch v := s.LitValue(l); v {
	case False:
		return false // conflicting assignment
	case True:
		return true // already assigned
	default:
		// New fact, store it.
		varID := l.VarID()
		s.assigns[l] = True
		s.assigns[l.Opposite()] = False
		s.level[varID] = s.decisionLevel()
		s.reason[varID] = from
		s.trail = append(s.trail, l)
		s.propQueue.Push(l)
		return true
	}
}

func (s *Solver) explain(c *Clause, l Literal) []Literal {
	if l == -1 {
		return c.ExplainFailure(s)
	} else {
		return c.ExplainAssign(s, l)
	}
}

func (s *Solver) analyze(confl *Clause) ([]Literal, int, int) {
	// Current number of "implication" nodes in exploration of the decision
	// level. A value of 0 indicates that the exploration has reached a single
	// implication point.
	nImplicationPoints := 0

	// Empty the buffer of literals in which the learnt clause will be stored.
	// Note position of the first literal is reserved for the FUIP.
	s.tmpLearnts = s.tmpLearnts[:0]
	s.tmpLearnts = append(s.tmpLearnts, -1)

	// Next literal to look at. This is used to iterate over the trail without
	// actually undoing the literal assignments.
	nextLiteral := len(s.trail) - 1

	l := Literal(-1) // unknown literal used to represent the conflict
	s.seenVar.Clear()
	backtrackLevel := 0

	for {
		for _, q := range s.explain(confl, l) {
			v := q.VarID()
			if s.seenVar.Contains(v) {
				continue
			}

			s.seenVar.Add(v)
			if s.level[v] == s.decisionLevel() {
				nImplicationPoints++
				continue
			}

			s.tmpLearnts = append(s.tmpLearnts, q.Opposite())
			if level := s.level[v]; level > backtrackLevel {
				backtrackLevel = level
			}
		}

		if confl.learnt && confl.lbd > 2 {
			// Opportunistically recompute the LBD of the clause as all its
			// literals are guaranteed to be assigned at this point.
			newLBD := s.computeLBD(confl.literals)

			// Clauses with an improving LBD are considered interesting and
			// worth protecting for a round.
			if newLBD < 30 && newLBD < confl.lbd {
				confl.isProtected = true
			}
			confl.lbd = newLBD
		}

		// Select next literal to look at.
		for {
			l = s.trail[nextLiteral]
			nextLiteral--
			v := l.VarID()
			confl = s.reason[v]
			if s.seenVar.Contains(v) {
				break
			}
		}

		nImplicationPoints--
		if nImplicationPoints <= 0 {
			break
		}
	}

	s.tmpLearnts[0] = l.Opposite()
	lbd := s.computeLBD(s.tmpLearnts)

	return s.tmpLearnts, lbd, backtrackLevel
}

// computeLBD returns the LBD (Literal Block Distance) of the given sequence of
// literals. All literals in the sequence must be assigned.
func (s *Solver) computeLBD(literals []Literal) int {
	lbd := 0
	s.seenLevel.Clear()
	for _, lit := range literals {
		l := s.level[lit.VarID()]
		if !s.seenLevel.Contains(l) {
			s.seenLevel.Add(l)
			lbd++
		}
	}
	return lbd
}

func (s *Solver) record(clause []Literal, lbd int) {
	c, _ := NewClause(s, clause, true)
	s.enqueue(clause[0], c)
	if c != nil {
		s.learnts = append(s.learnts, c)
		c.lbd = lbd
	}
}

func (s *Solver) Search(nConflicts int) LBool {
	if s.unsat {
		return False
	}

	s.TotalRestarts++
	conflictCount := 0

	for !s.shouldStop() {
		if s.TotalIterations%100000 == 0 {
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

			learntClause, lbd, backtrackLevel := s.analyze(conflict)
			s.cancelUntil(backtrackLevel)

			s.record(learntClause, lbd)

			s.DecayClaActivity()
			s.DecayVarActivity()

			continue
		}

		// No Conflict
		// -----------

		if s.TotalConflicts >= s.nextReduce {
			s.nextReduce += (s.nextReduceIncr) * s.TotalRestarts
			s.ReduceDB()
		}

		if s.decisionLevel() == 0 {
			s.Simplify()
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

	s.order.Undo(v)
	s.assigns[l] = Unknown
	s.assigns[l.Opposite()] = Unknown
	s.reason[v] = nil
	s.level[v] = -1

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
	model := make([]bool, s.NumVariables())
	for i := range model {
		lb := s.VarValue(i)
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
