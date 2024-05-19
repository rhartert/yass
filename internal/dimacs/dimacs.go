package dimacs

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"

	"github.com/rhartert/dimacs"
	"github.com/rhartert/yass/internal/sat"
)

type satSolver interface {
	AddVariable() int
	AddClause([]sat.Literal) error
}

func reader(filename string, gzipped bool) (io.ReadCloser, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	rc := io.ReadCloser(file)
	if gzipped {
		rc, err = gzip.NewReader(rc)
		if err != nil {
			return nil, err
		}
	}
	return rc, nil
}

// builder wraps the solver to implement dimacs.Builder.
type builder struct {
	solver satSolver
}

func (b *builder) Problem(problem string, nVars int, nClauses int) error {
	if problem != "cnf" {
		return fmt.Errorf("not a CNF problem")
	}
	for i := 0; i < nVars; i++ {
		b.solver.AddVariable()
	}
	return nil
}

func (b *builder) Clause(tmpClause []int) error {
	clause := make([]sat.Literal, len(tmpClause))
	for i, l := range tmpClause {
		if l < 0 {
			clause[i] = sat.NegativeLiteral(-l - 1)
		} else {
			clause[i] = sat.PositiveLiteral(l - 1)
		}
	}
	b.solver.AddClause(clause)
	return nil
}

func (b *builder) Comment(_ string) error {
	return nil // ignore comments
}

func LoadDIMACS(filename string, gzipped bool, solver satSolver) error {
	reader, err := reader(filename, gzipped)
	if err != nil {
		return fmt.Errorf("error reading file %q: %s", filename, err)
	}
	defer reader.Close()

	b := &builder{solver}
	return dimacs.ReadBuilder(reader, b)
}
