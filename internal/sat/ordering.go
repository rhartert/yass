package sat

import (
	"log"

	"github.com/rhartert/yagh"
)

type VarOrder struct {
	size   int
	heap   *yagh.IntMap[float64]
	solver *Solver
}

func NewVarOrder(s *Solver, nVar int) *VarOrder {
	vo := &VarOrder{
		size:   nVar,
		solver: s,
		heap:   yagh.New[float64](nVar),
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

		return vo.solver.NegativeLiteral(next.Elem)
	}
}
