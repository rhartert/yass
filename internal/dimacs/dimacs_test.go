package dimacs

import (
	_ "embed"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rhartert/yass/internal/sat"
)

type instance struct {
	Variables int
	Clauses   [][]sat.Literal
}

func (i *instance) AddVariable() int {
	i.Variables++
	return i.Variables - 1
}

func (i *instance) AddClause(tmpClause []sat.Literal) error {
	clause := make([]sat.Literal, len(tmpClause))
	copy(clause, tmpClause)
	i.Clauses = append(i.Clauses, clause)
	return nil
}

var want = instance{
	Variables: 3,
	Clauses: [][]sat.Literal{
		{0, 2, 4},
		{0, 2, 5},
		{0, 3, 4},
		{1, 2, 4},
		{1, 3, 4},
		{1, 2, 5},
		{0, 3, 5},
		{1, 3, 5},
	},
}

func TestParseDIMACS_cnf(t *testing.T) {
	got := instance{}
	gotErr := LoadDIMACS("testdata/test_instance.cnf", false, &got)

	if gotErr != nil {
		t.Errorf("ParseDIMACS(): want no error, got %s", gotErr)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ParseDIMACS(): mismatch (+want, -got):\n%s", diff)
	}
}

func TestParseDIMACS_gzip(t *testing.T) {
	got := instance{}
	gotErr := LoadDIMACS("testdata/test_instance.cnf.gz", true, &got)

	if gotErr != nil {
		t.Errorf("ParseDIMACS(): want no error, got %s", gotErr)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ParseDIMACS(): mismatch (+want, -got):\n%s", diff)
	}
}

func TestParseDIMACS_noFile(t *testing.T) {
	got := instance{}
	gotErr := LoadDIMACS("", false, &got)

	if gotErr == nil {
		t.Errorf("ParseDIMACS(): want error, got none")
	}
}

func TestParseDIMACS_gzip_notGzipFile(t *testing.T) {
	got := instance{}
	gotErr := LoadDIMACS("testdata/test_instance.cnf", true, &got)

	if gotErr == nil {
		t.Errorf("ParseDIMACS(): want error, got none")
	}
}
