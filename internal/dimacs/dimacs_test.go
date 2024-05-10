package dimacs

import (
	_ "embed"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var testInstance = Instance{
	Variables: 3,
	Clauses: [][]int{
		{1, 2, 3},
		{1, 2, -3},
		{1, -2, 3},
		{-1, 2, 3},
		{-1, -2, 3},
		{-1, 2, -3},
		{1, -2, -3},
		{-1, -2, -3},
	},
	Comments: []string{"c minialist unsat instance"},
}

func TestParseDIMACS_cnf(t *testing.T) {
	want := &testInstance

	got, err := ParseDIMACS("testdata/test_instance.cnf", false)

	if err != nil {
		t.Errorf("ParseDIMACS(): want no error, got %s", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ParseDIMACS(): mismatch (+want, -got):\n%s", diff)
	}
}

func TestParseDIMACS_gzip(t *testing.T) {
	want := &testInstance

	got, err := ParseDIMACS("testdata/test_instance.cnf.gz", true)

	if err != nil {
		t.Errorf("ParseDIMACS(): want no error, got %s", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ParseDIMACS(): mismatch (+want, -got):\n%s", diff)
	}
}

func TestParseDIMACS_noFile(t *testing.T) {
	got, err := ParseDIMACS("", false)

	if err == nil {
		t.Errorf("ParseDIMACS(): want error, got none")
	}
	if got != nil {
		t.Errorf("ParseDIMACS(): want nil instance, got %+v", got)
	}
}

func TestParseDIMACS_gzip_notGzipFile(t *testing.T) {
	got, err := ParseDIMACS("testdata/test_instance.cnf", true)

	if err == nil {
		t.Errorf("ParseDIMACS(): want error, got none")
	}
	if got != nil {
		t.Errorf("ParseDIMACS(): want nil instance, got %+v", got)
	}
}
