package sat

import (
	"log"

	"github.com/rhartert/yagh"
)

// VarOrder maintains the order of variable to be assigned by the solver.
type VarOrder struct {
	// Binary heap to access the next variable with the highest score. The heap
	// breaks ties using the index of its elements which will correspond to the
	// order in which variables are declared with AddVar.
	order *yagh.IntMap[float64]

	scores     []float64 // in [0, 1e100)
	scoreInc   float64   // in (0, 1e100)
	scoreDecay float64   // in (0, 1]

	phases      []LBool
	phaseSaving bool
}

// NewVarOrder returns a new initialized VarOrder.
func NewVarOrder(decay float64, phaseSaving bool) *VarOrder {
	return &VarOrder{
		order:       yagh.New[float64](0),
		scoreInc:    1,
		scoreDecay:  decay,
		phases:      make([]LBool, 0),
		phaseSaving: phaseSaving,
	}
}

// AddVar adds a new variable with the given inital score and phase.
func (vo *VarOrder) AddVar(initScore float64, initPhase bool) {
	varID := len(vo.phases)

	vo.scores = append(vo.scores, initScore)
	vo.phases = append(vo.phases, Lift(initPhase))

	vo.order.GrowBy(1)
	vo.order.Put(varID, -initScore)
}

// Reinsert adds variable v back to the set of candidates to be selected. This
// function must be called by the solver when v is being unassigned (e.g. when
// a backtrack occurs) where val is the value the variable was assigned to.
func (vo *VarOrder) Reinsert(v int, val LBool) {
	if vo.phaseSaving {
		vo.phases[v] = val
	}
	act := vo.scores[v]
	vo.order.Put(v, -act)
}

// DecayScores slightly decreases the scores of the variables. This is used
// to give more importance to variables that have had their scores increased
// recently compared to variables that had their scores increased in the past.
func (vo *VarOrder) DecayScores() {
	vo.scoreInc /= vo.scoreDecay // decay activities by bumping increment
	if vo.scoreInc > 1e100 {
		vo.rescaleScoresAndIncrement()
	}
}

// BumpScore increases the score of the given variable. Note that this operation
// might trigger a rescaling of all variables scores if the score of v exceeds
// a given threshold. The rescaling is done in way that conserves the relative
// importance of each variable when compared to each other.
func (vo *VarOrder) BumpScore(v int) {
	newScore := vo.scores[v] + vo.scoreInc
	vo.scores[v] = newScore
	if vo.order.Contains(v) {
		vo.order.Put(v, -newScore)
	}
	if vo.scores[v] > 1e100 {
		vo.rescaleScoresAndIncrement()
	}
}

// NextDecision returns the next unnassigned literal to be assigned to true.
func (vo *VarOrder) NextDecision(s *Solver) Literal {
	for {
		next, ok := vo.order.Pop()
		if !ok {
			log.Fatalln("empty heap")
		}
		if s.VarValue(next.Elem) != Unknown {
			continue // already assigned
		}

		switch vo.phases[next.Elem] {
		case True:
			return PositiveLiteral(next.Elem)
		case False:
			return NegativeLiteral(next.Elem)
		default:
			return PositiveLiteral(next.Elem)
		}
	}
}

func (vo *VarOrder) rescaleScoresAndIncrement() {
	vo.scoreInc *= 1e-100 // important to keep proportions
	for v, s := range vo.scores {
		newScore := s * 1e-100
		vo.scores[v] = newScore
		if vo.order.Contains(v) {
			vo.order.Put(v, -newScore)
		}
	}
}
