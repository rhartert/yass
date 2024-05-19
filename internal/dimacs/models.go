package dimacs

import (
	"fmt"
	"os"

	"github.com/rhartert/dimacs"
)

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

func ParseModels(filename string) ([][]bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	b := &modelBuilder{}
	if err := dimacs.ReadBuilder(file, b); err != nil {
		return nil, err
	}

	return b.models, nil
}
