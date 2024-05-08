package dimacs

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rhartert/yass/internal/sat"
)

type Instance struct {
	Variables int
	Clauses   [][]int
	Comments  []string
}

func ParseDIMACS(filename string) (*Instance, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	instance := &Instance{}
	scanner := bufio.NewScanner(file)
	stop := false
	for i := 0; scanner.Scan() && !stop; i++ {
		line := scanner.Text()
		if line == "" {
			continue
		}

		switch line[0] {
		case '%': // end of instance
			stop = true
		case 'c':
			if err := parseCommentLine(instance, line); err != nil {
				return nil, err
			}
		case 'p':
			if err := parseHeaderLine(instance, line); err != nil {
				return nil, err
			}
		default:
			if err := parseClauseLine(instance, line); err != nil {
				return nil, err
			}
		}
	}

	return instance, nil
}

// Instantiate adds the instance's variables and clauses to solver s.
func Instantiate(s *sat.Solver, instance *Instance) error {
	for i := 0; i < instance.Variables; i++ {
		s.AddVariable()
	}
	for _, c := range instance.Clauses {
		clause := []sat.Literal{}
		for _, v := range c {
			if v < 0 {
				clause = append(clause, sat.NegativeLiteral(-v-1))
			} else {
				clause = append(clause, sat.PositiveLiteral(v-1))
			}
		}
		s.AddClause(clause)
	}

	return nil
}

func parseCommentLine(instance *Instance, line string) error {
	instance.Comments = append(instance.Comments, line)
	return nil
}

func parseHeaderLine(instance *Instance, line string) error {
	if instance.Clauses != nil {
		return fmt.Errorf("found a second header line %q", line)
	}
	parts := strings.Fields(line)
	if parts[1] != "cnf" {
		return fmt.Errorf("instance of type %q are not supported", parts[1])
	}
	nVar, err := strconv.Atoi(parts[2])
	if err != nil {
		return fmt.Errorf("could not parse header: %w", err)
	}
	instance.Variables = nVar
	nClauses, err := strconv.Atoi(parts[3])
	if err != nil {
		return fmt.Errorf("could not parse header: %w", err)
	}
	instance.Clauses = make([][]int, 0, nClauses)
	return nil
}

func parseClauseLine(instance *Instance, line string) error {
	if instance.Clauses == nil {
		return fmt.Errorf("found clause line before header %q", line)
	}
	c, err := parseClause(line)
	if err != nil {
		return fmt.Errorf("could not parse clause %q: %w", line, err)
	}
	instance.Clauses = append(instance.Clauses, c)
	return nil
}

func parseClause(line string) ([]int, error) {
	parts := strings.Fields(line)
	literals := make([]int, len(parts)-1)
	for i, p := range parts[:len(literals)] {
		l, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		literals[i] = l
	}
	return literals, nil
}
