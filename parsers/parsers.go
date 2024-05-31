package parsers

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"

	"github.com/rhartert/dimacs"
	"github.com/rhartert/yass/sat"
)

type SATSolver interface {
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

// LoadDIMACS parses the DIMACS CNF file and loads its CNF formula in the
// given SAT solver.
func LoadDIMACS(filename string, gzipped bool, solver SATSolver) error {
	reader, err := reader(filename, gzipped)
	if err != nil {
		return fmt.Errorf("error reading file %q: %s", filename, err)
	}
	defer reader.Close()

	b := &builder{solver}
	return dimacs.ReadBuilder(reader, b)
}

// builder wraps the solver to implement dimacs.Builder.
type builder struct {
	solver SATSolver
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

// ReadModels returns the list of models (if any) contained in the given file.
func ReadModels(filename string) ([][]bool, error) {
	reader, err := reader(filename, false)
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %s", filename, err)
	}
	defer reader.Close()

	b := &modelBuilder{}
	if err := dimacs.ReadBuilder(reader, b); err != nil {
		return nil, err
	}

	return b.models, nil
}

// builder wraps the solver to implement dimacs.Builder.
type modelBuilder struct {
	models [][]bool
}

func (b *modelBuilder) Problem(problem string, nVars int, nClauses int) error {
	return fmt.Errorf("model files should not have problem lines")
}

func (b *modelBuilder) Comment(_ string) error {
	return nil // ignore comments
}

func (b *modelBuilder) Clause(tmpClause []int) error {
	model := make([]bool, len(tmpClause))
	for i, l := range tmpClause {
		model[i] = l > 0
	}
	b.models = append(b.models, model)
	return nil
}
