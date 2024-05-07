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

	// Variable ordering.
	activities  []float64
	varInc      float64
	varDecay    float64
	order       *VarOrder
	phaseSaving bool

	// Propagation and watchers.
	watchers  [][]watcher
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

	// Shared by operation that needs to put variables in a set and empty that
	// set efficiently.
	seenVar *ResetSet

	// Temporary slice used in the Propagate function. The slice is re-used by
	// all Propagate calls to avoid unnecessarily allocating new slices.
	tmpWatchers []watcher

	// Temporary slice used in Analyze to accumulate literals before these are
	// used to create a new learnt clause. Having one shared buffer between all
	// call reduces the overhead of having to grow each time Analye is called.
	tmpLearnts []Literal

	// Used for clause to explain themselves.
	tmpReason []Literal
}

// watcher represents a clause attached to the watch list of a literal.
type watcher struct {
	// The watching clause to be propagated when the watched literal becomes
	// true.
	clause *Clause

	// Guard is one of the clause's literals. If it is true, then there is
	// no need to propagate the clause. Note that the guard literal must be
	// different from the watcher literal.
	guard Literal
}

type Options struct {
	ClauseDecay   float64
	VariableDecay float64
	MaxConflicts  int64
	Timeout       time.Duration
	PhaseSaving   bool
}

var DefaultOptions = Options{
	ClauseDecay:   0.999,
	VariableDecay: 0.95,
	MaxConflicts:  -1,
	Timeout:       -1,
	PhaseSaving:   false,
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
		clauseInc:   1,
		varInc:      1,
		propQueue:   NewQueue[Literal](128),
		maxConflict: -1,
		timeout:     -1,
		seenVar:     &ResetSet{},
		phaseSaving: ops.PhaseSaving,
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

	// One for each literal.
	s.assigns = append(s.assigns, Unknown)
	s.assigns = append(s.assigns, Unknown)

	s.level = append(s.level, -1)
	s.activities = append(s.activities, 0)
	s.order.NewVar()
	return index
}

// Watch registers clause c to be awaken when Literal watch is assigned to true.
func (s *Solver) Watch(c *Clause, watch Literal, guard Literal) {
	s.watchers[watch] = append(s.watchers[watch], watcher{
		clause: c,
		guard:  guard,
	})
}

// Unwatch removes clause c from the list of watchers.
func (s *Solver) Unwatch(c *Clause, watch Literal) {
	j := 0
	for i := 0; i < len(s.watchers[watch]); i++ {
		if s.watchers[watch][i].clause != c {
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
	s.order = NewVarOrder(s, s.NumVariables())
	s.order.phaseSaving = s.phaseSaving
	s.startTime = time.Now()

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
		s.clauseInc *= 1e-100 // important to keep proportions
		for _, l := range s.learnts {
			l.activity *= 1e-100
		}
	}
}

func (s *Solver) BumpVarActivity(l Literal) {
	vid := l.VarID()
	s.activities[vid] += s.varInc

	if s.activities[vid] > 1e100 {
		s.varInc *= 1e-100 // important to keep proportions
		for i := range s.activities {
			s.activities[i] *= 1e-100
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

		for i, w := range s.tmpWatchers {
			// No need to propagate the clause if its guard is true. This block
			// is not necessary for propagation to behave properly. However, it
			// helps to significantly speed-up computation by avoiding loading
			// clause (in memory) that do not need to be propagated. Note that
			// this alters the order in which clause are propagated and can thus
			// yield to different conflict analysis and learnt clauses.
			if s.LitValue(w.guard) == True {
				s.watchers[l] = append(s.watchers[l], w)
				continue
			}

			if w.clause.Propagate(s, l) {
				continue
			}

			// Constraint is conflicting, copy remaining watchers
			// and return the constraint.
			s.watchers[l] = append(s.watchers[l], s.tmpWatchers[i+1:]...)
			s.propQueue.Clear()
			return s.tmpWatchers[i].clause
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

func (s *Solver) analyze(confl *Clause) ([]Literal, int) {
	// Current number of "implication" nodes encountered in the exploration of
	// the decision level. A value of 0 indicates that the exploration has
	// reached a single implication point.
	nImplicationPoints := 0

	// Empty the buffer of literals in which the learnt clause will be stored.
	// Note that the first literal is reserved for the FUIP which is set at the
	// of this function.
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

	// Add literal corresponding to the FUIP.
	s.tmpLearnts[0] = l.Opposite()

	return s.tmpLearnts, backtrackLevel
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
			s.cancelUntil(backtrackLevel)

			s.record(learntClause)

			s.DecayClaActivity()
			s.DecayVarActivity()

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
