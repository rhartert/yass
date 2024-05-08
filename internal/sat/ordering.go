package sat

import (
	"log"

	"github.com/rhartert/yagh"
)

type VarOrder struct {
	order *yagh.IntMap[float64]

	scores     []float64
	scoreInc   float64
	scoreDecay float64

	phases      []LBool
	phaseSaving bool
}

func NewVarOrder(decay float64, phaseSaving bool) *VarOrder {
	return &VarOrder{
		order:       yagh.New[float64](0),
		scoreInc:    1,
		scoreDecay:  decay,
		phases:      make([]LBool, 0),
		phaseSaving: phaseSaving,
	}
}

func (vo *VarOrder) AddVar(initScore float64, initPhase LBool) {
	varID := len(vo.phases)

	vo.scores = append(vo.scores, initScore)
	vo.phases = append(vo.phases, initPhase)

	vo.order.GrowBy(1)
	vo.order.Put(varID, -initScore)
}

// Reinsert adds variable b back to the list of candidates to be selected. This
// function must be called when v is being unassigned (e.g. when a backtrack
// occurs) where val is the value the variable was assigned to.
func (vo *VarOrder) Reinsert(v int, val LBool) {
	if vo.phaseSaving {
		vo.phases[v] = val
	}
	act := vo.scores[v]
	vo.order.Put(v, -act)
}

func (vo *VarOrder) DecayScores() {
	vo.scoreInc /= vo.scoreDecay // decay activities by bumping increment
	if vo.scoreInc > 1e100 {
		vo.rescaleScoresAndIncrement()
	}
}

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
			return NegativeLiteral(next.Elem)
		}
	}
}
