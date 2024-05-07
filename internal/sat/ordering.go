package sat

import (
	"log"

	"github.com/rhartert/yagh"
)

type VarOrder struct {
	size        int
	solver      *Solver
	phase       []LBool
	phaseSaving bool
	heap        *yagh.IntMap[float64]
}

func NewVarOrder(s *Solver, nVar int) *VarOrder {
	vo := &VarOrder{
		size:        nVar,
		solver:      s,
		phase:       make([]LBool, nVar),
		phaseSaving: false,
		heap:        yagh.New[float64](nVar),
	}

	vo.UpdateAll()
	return vo
}

func (vo *VarOrder) NewVar() {}

func (vo *VarOrder) Update(varID int) {
	if vo.heap.Contains(varID) {
		vo.Undo(varID)
	}
}

func (vo *VarOrder) UpdateAll() {
	for i := 0; i < vo.size; i++ {
		vo.Undo(i)
	}
}

func (vo *VarOrder) Undo(varID int) {
	if vo.phaseSaving {
		vo.phase[varID] = vo.solver.VarValue(varID)
	}

	act := vo.solver.activities[varID]
	vo.heap.Put(varID, -act)
}

func (vo *VarOrder) Select() Literal {
	for {
		next, ok := vo.heap.Pop()
		if !ok {
			log.Fatalln("empty heap")
		}
		if vo.solver.VarValue(next.Elem) != Unknown {
			continue // already assigned
		}

		switch vo.phase[next.Elem] {
		case True:
			return vo.solver.PositiveLiteral(next.Elem)
		case False:
			return vo.solver.NegativeLiteral(next.Elem)
		default:
			return vo.solver.NegativeLiteral(next.Elem)
		}
	}
}
