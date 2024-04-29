package dimacs

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func ParseModels(filename string) ([][]bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	models := [][]bool{}
	scanner := bufio.NewScanner(file)
	for i := 0; scanner.Scan(); i++ {
		line := scanner.Text()
		if line == "" {
			continue
		}

		literals := strings.Fields(line)
		model := make([]bool, 0, len(literals))

		for _, ls := range literals {
			if ls == "0" {
				continue
			}
			l, err := strconv.Atoi(ls)
			if err != nil {
				return nil, fmt.Errorf("error parsing literal %s: %w", ls, err)
			}
			model = append(model, l > 0)
		}

		models = append(models, model)
	}

	return models, nil
}
