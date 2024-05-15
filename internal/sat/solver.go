package sat

import (
	"fmt"
	"log"
	"sort"
	"time"
)

type Statistics struct {
	Propagations uint64
	Guards       uint64
	Conflicts    uint64
	Iterations   uint64
	Decisions    uint64
	Restarts     uint64
}

type Solver struct {
	// Variable ordering.
	order *VarOrder

	// Whether the solver has reached a top level conflict or not.
	unsat bool

	// Value assigned to each literal.
	assigns []LBool

	// Clause responsible for assigning a variable (nil if unnassigned)
	assignReasons []*Clause

	// Level at which each variable was assigned (-1 if unnassigned).
	assignLevels []int

	// Clause database.
	constraints []*Clause
	learnts     []*Clause
	clauseInc   float64
	clauseDecay float64

	// Threshold in terms of total number of conflicts after which a reduction
	// of the clause DB is triggered. This value is adapted dynamically during
	// search (see below).
	conflictBeforeReduce uint64

	// Number of conflicts by which the above threshold is increased after each
	// reduction of the clause DB. That increment itself is increased by
	// conflictBeforeReduceIncInc after each reduction.
	conflictBeforeReduceInc    uint64
	conflictBeforeReduceIncInc uint64

	// List of watcher for each literal.
	watchers [][]watcher

	// Queue of recently assigned literal that still have to be propagated.
	propQueue *Queue[Literal]

	// Trail.
	trail    []Literal
	trailLim []int

	// Search statistics.
	Statistics Statistics

	// Stop conditions.
	startTime   time.Time
	hasStopCond bool
	maxConflict int64
	timeout     time.Duration

	// Models.
	Models [][]bool

	// Temporary slice used in the Propagate function. The slice is re-used by
	// all Propagate calls to avoid unnecessarily allocating new slices.
	tmpWatchers []watcher

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
		clauseDecay:                ops.ClauseDecay,
		clauseInc:                  1,
		order:                      NewVarOrder(ops.VariableDecay, ops.PhaseSaving),
		propQueue:                  NewQueue[Literal](128),
		maxConflict:                -1,
		timeout:                    -1,
		conflictBeforeReduceInc:    2000,
		conflictBeforeReduceIncInc: 300,
		seenLevel:                  &ResetSet{},
		seenVar:                    &ResetSet{},
		tmpLearnts:                 make([]Literal, 0, 32),
		tmpReason:                  make([]Literal, 0, 32),
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
	if s.maxConflict >= 0 && uint64(s.maxConflict) <= s.Statistics.Conflicts {
		return true
	}
	if s.timeout >= 0 && s.timeout <= time.Since(s.startTime) {
		return true
	}

	return false
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
	return s.assigns[PositiveLiteral(x)]
}

func (s *Solver) LitValue(l Literal) LBool {
	return s.assigns[l]
}

func (s *Solver) AddVariable() int {
	index := s.NumVariables()
	s.watchers = append(s.watchers, nil)
	s.watchers = append(s.watchers, nil)

	s.seenVar.Expand()
	s.seenLevel.Expand()

	s.assignReasons = append(s.assignReasons, nil)
	s.assignLevels = append(s.assignLevels, -1)
	s.assigns = append(s.assigns, Unknown, Unknown) // one for each literal

	s.order.AddVar(0.0, true)
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
	for _, w := range s.watchers[watch] {
		if w.clause != c {
			s.watchers[watch][j] = w
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
	for _, c := range clauses {
		if c.Simplify(s) {
			c.Delete(s)
		} else {
			clauses[j] = c
			j++
		}
	}
	*clausesPtr = clauses[:j]
}

func (s *Solver) decisionLevel() int {
	return len(s.trailLim)
}

func (s *Solver) Solve() LBool {
	numConflicts := uint64(100)
	status := Unknown

	s.startTime = time.Now()
	s.Statistics = Statistics{}

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

	s.backtrackTo(0)
	return status
}

func (s *Solver) BumpClaActivity(c *Clause) {
	c.activity += s.clauseInc
	if c.activity > 1e100 {
		s.rescaleClauseActivitiesAndIncrement()
	}
}

func (s *Solver) DecayClaActivity() {
	s.clauseInc /= s.clauseDecay // decay activities by bumping increment
	if s.clauseInc > 1e100 {
		s.rescaleClauseActivitiesAndIncrement()
	}
}

func (s *Solver) rescaleClauseActivitiesAndIncrement() {
	s.clauseInc *= 1e-100 // important to keep proportions
	for _, l := range s.learnts {
		l.activity *= 1e-100
	}
}

func (s *Solver) Propagate() *Clause {
	for s.propQueue.Size() > 0 {
		l := s.propQueue.Pop()

		s.tmpWatchers = s.tmpWatchers[:0]
		s.tmpWatchers = append(s.tmpWatchers, s.watchers[l]...)
		s.watchers[l] = s.watchers[l][:0]

		for i, w := range s.tmpWatchers {
			s.Statistics.Propagations++

			// No need to propagate the clause if its guard is true. This block
			// is not necessary for propagation to behave properly. However, it
			// helps to significantly speed-up computation by avoiding loading
			// clause (in memory) that do not need to be propagated. Note that
			// this alters the order in which clause are propagated and can thus
			// yield to different conflict analysis and learnt clauses.
			if s.LitValue(w.guard) == True {
				s.Statistics.Guards++
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
		s.assignLevels[varID] = s.decisionLevel()
		s.assignReasons[varID] = from
		s.trail = append(s.trail, l)
		s.propQueue.Push(l)
		return true
	}
}

func (s *Solver) analyze(conflicting *Clause) ([]Literal, int, int) {
	// Current number of "implication" nodes encountered in the exploration of
	// the decision level. A value of 0 indicates that the exploration has
	// reached a single implication point.
	nImplicationPoints := 0

	// Empty the buffer of literals in which the learnt clause will be stored.
	// Note that the first literal is reserved for the FUIP which is set at the
	// end of this function.
	s.tmpLearnts = s.tmpLearnts[:0]
	s.tmpLearnts = append(s.tmpLearnts, 0)

	// Clause to generate an explanation, starting with the conflicting clause.
	c := conflicting

	// Position of the next literal on the trail to be inspected. Note that
	// no literal is inspected in the first iteration of the analysis loop as
	// it focuses on explaining the conflict.
	trailTop := len(s.trail)

	s.seenVar.Clear()
	backtrackLevel := 0

	for {
		if c == conflicting {
			c.explainConflict(&s.tmpReason)
		} else {
			c.explainAssign(&s.tmpReason)
		}
		if c.isLearnt() {
			s.BumpClaActivity(c)
		}

		for _, q := range s.tmpReason {
			v := q.VarID()
			if s.seenVar.Contains(v) {
				continue
			}

			s.seenVar.Add(v)

			level := s.assignLevels[v]
			if level == s.decisionLevel() {
				nImplicationPoints++
				continue
			}

			backtrackLevel = max(backtrackLevel, level)
			s.tmpLearnts = append(s.tmpLearnts, q.Opposite())
		}

		if c.isLearnt() && c.lbd > 2 {
			// Opportunistically recompute the LBD of the clause as all its
			// literals are guaranteed to be assigned at this point.
			newLBD := s.computeLBD(c.literals)

			// Clauses with an improving LBD are considered interesting and
			// worth protecting for a round.
			if newLBD < 30 && newLBD < c.lbd {
				c.setProtected()
			}
			c.lbd = newLBD
		}

		// Select next literal to look at.
		for {
			trailTop--
			v := s.trail[trailTop].VarID()
			c = s.assignReasons[v]
			if s.seenVar.Contains(v) {
				break
			}
		}

		nImplicationPoints--
		if nImplicationPoints <= 0 {
			break
		}
	}

	s.tmpLearnts[0] = s.trail[trailTop].Opposite()
	lbd := s.computeLBD(s.tmpLearnts)

	return s.tmpLearnts, lbd, backtrackLevel
}

// computeLBD returns the LBD (Literal Block Distance) of the given sequence of
// literals. All literals in the sequence must be assigned.
func (s *Solver) computeLBD(literals []Literal) int {
	lbd := 0
	s.seenLevel.Clear()
	s.seenLevel.Add(0)
	for _, lit := range literals {
		l := s.assignLevels[lit.VarID()]
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
		s.BumpClaActivity(c)
		for _, l := range c.literals {
			s.order.BumpScore(l.VarID())
		}

		s.learnts = append(s.learnts, c)
		c.lbd = lbd
	}
}

func (s *Solver) Search(nConflicts uint64) LBool {
	s.Statistics.Restarts++

	if s.unsat {
		return False
	}

	conflictLimit := s.Statistics.Conflicts + nConflicts

	for !s.shouldStop() {
		if s.Statistics.Iterations%100000 == 0 {
			s.printSearchStats()
		}
		s.Statistics.Iterations++

		if conflict := s.Propagate(); conflict != nil {
			s.Statistics.Conflicts++

			if s.decisionLevel() == 0 {
				s.unsat = true
				return False
			}

			learntClause, lbd, backtrackLevel := s.analyze(conflict)
			s.backtrackTo(backtrackLevel)

			s.record(learntClause, lbd)

			s.DecayClaActivity()
			s.order.DecayScores()

			continue
		}

		// No Conflict
		// -----------

		if s.decisionLevel() == 0 {
			s.Simplify()
		}

		if s.Statistics.Conflicts >= s.conflictBeforeReduce {
			s.conflictBeforeReduceInc += s.conflictBeforeReduceIncInc
			s.conflictBeforeReduce += s.conflictBeforeReduceInc
			s.ReduceDB()
		}

		if s.NumAssigns() == s.NumVariables() { // solution found
			s.saveModel()
			s.backtrackTo(0)
			return True
		}

		if s.Statistics.Conflicts > conflictLimit {
			s.backtrackTo(0)
			return Unknown
		}

		l := s.order.NextDecision(s)
		s.assume(l)
	}

	return Unknown
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
		s.learnts[i].setProtected()
	}

	toDelete := len(s.learnts) / 2

	i, j := 0, 0
	for ; i < len(s.learnts); i++ {
		c := s.learnts[i]

		if toDelete > 0 && !c.locked(s) && c.lbd > 2 && len(c.literals) > 2 && !c.isProtected() {
			toDelete--
			c.Delete(s)
		} else {
			if c.isProtected() {
				c.setUnprotected()
				toDelete++
			}
			s.learnts[j] = s.learnts[i]
			j++
		}
	}

	s.learnts = s.learnts[:j]
}

func (s *Solver) backtrackTo(level int) {
	for s.decisionLevel() > level {
		c := len(s.trail) - s.trailLim[len(s.trailLim)-1]
		for ; c != 0; c-- {
			s.unnassignedLast()
		}
		s.trailLim = s.trailLim[:len(s.trailLim)-1]
	}
}

func (s *Solver) unnassignedLast() {
	l := s.trail[len(s.trail)-1]
	v := l.VarID()

	s.order.Reinsert(v, s.VarValue(v))
	s.assigns[l] = Unknown
	s.assigns[l.Opposite()] = Unknown
	s.assignReasons[v] = nil
	s.assignLevels[v] = -1

	s.trail = s.trail[:len(s.trail)-1]
}

func (s *Solver) assume(l Literal) bool {
	s.trailLim = append(s.trailLim, len(s.trail))
	return s.enqueue(l, nil)
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
		s.Statistics.Iterations,
		s.Statistics.Conflicts,
		s.Statistics.Restarts,
		len(s.learnts))
}
