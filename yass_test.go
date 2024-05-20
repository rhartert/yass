package main

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rhartert/yass/internal/parsers"
	"github.com/rhartert/yass/internal/sat"
)

// This test suite evaluates the correctness of YASS by verifying that the
// solver is able to find the exact set of models for each instance in a
// comprehensive set of instances (see testdataDir).
//
// The test set includes instances with known solutions, which have been
// pre-computed using trusted reference SAT solvers such as [MiniSAT] and
// [Glucose].
//
// [MiniSAT]: http://minisat.se/
// [Glucose]: https://www.labri.fr/perso/lsimon/research/glucose/

// Directory containing the test cases used to validate YASS. Each test case
// must be provided with two files:
//
//   - An instance file containing a valid DIMACS SAT/UNSAT instance with the
//     ".cnf" file extension.
//   - A models file containing the (possibly empty) set of instance's models.
//     The file must contain one model per line using the same literals as in
//     the corresponding instance file. The models file must have the same name
//     as the instance file but with the ".cnf.models" file extension.
//
// Note that the test directory can contain subdirectories.
var testdataDir = "testdata"

type testCase struct {
	instanceName string
	instanceFile string
	modelsFile   string
}

// listTestCases returns the list of test cases contained in the file tree
// rooted in the given directory.
func listTestCases(dir string) ([]testCase, error) {
	testCases := []testCase{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".cnf") {
			return nil // not an instance file
		}
		testCases = append(testCases, testCase{
			instanceName: d.Name(),
			instanceFile: path,
			modelsFile:   path + ".models",
		})
		return nil
	})

	return testCases, err
}

// toString returns a binary string representation of the given model. For
// example, model [true, false, false] results in string "100".
func toString(model []bool) string {
	s := make([]byte, 0, len(model))
	for _, b := range model {
		if b {
			s = append(s, 1)
		} else {
			s = append(s, 0)
		}
	}
	return string(s)
}

// toSet converts a slice of models into a set of models represented as binary
// strings (see toString).
func toSet(s [][]bool) map[string]struct{} {
	set := map[string]struct{}{}
	for _, m := range s {
		set[toString(m)] = struct{}{}
	}
	return set
}

// solveAll returns an unordered list of all the instance's models.
func solveAll(s *sat.Solver) [][]bool {
	for s.Solve() == sat.True {
		// Add a new clause to forbid the last model found. Note that literal
		// must be flipped: !(a ^ b ^ c) corresponds to (!a v !b v !c).
		modelClause := make([]sat.Literal, s.NumVariables())
		for i, b := range s.Models[len(s.Models)-1] {
			if b { // literals are flipped
				modelClause[i] = sat.NegativeLiteral(i)
			} else {
				modelClause[i] = sat.PositiveLiteral(i)
			}
		}
		s.AddClause(modelClause)
	}
	return s.Models
}

// TestSolveAll verifies that the solver is able to find all the models of a
// set of instances. Test cases (i.e. instances) are evaluated in parallel.
func TestSolveAll(t *testing.T) {
	testCases, err := listTestCases(testdataDir)
	if err != nil {
		t.Fatalf("Error parsing test cases: %s", err)
	}

	for i := 0; i < len(testCases); i++ {
		tc := testCases[i]
		t.Run(tc.instanceName, func(t *testing.T) {
			t.Parallel()

			want, err := parsers.ReadModels(tc.modelsFile)
			if err != nil {
				t.Errorf("Model parsing error: %s", err)
			}
			s := sat.NewDefaultSolver()
			if err := parsers.LoadDIMACS(tc.instanceFile, false, s); err != nil {
				t.Errorf("Instance parsing error: %s", err)
			}

			got := solveAll(s)

			if len(got) != len(want) {
				t.Errorf("Incorrect number of models: got %d, want %d", len(got), len(want))
			}
			if !cmp.Equal(toSet(got), toSet(want)) {
				t.Errorf("Model mismatch")
			}
		})
	}
}
