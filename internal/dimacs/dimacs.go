package dimacs

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/rhartert/yass/internal/sat"
)

type dimacsWritter interface {
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

func LoadDIMACS(filename string, gzipped bool, dw dimacsWritter) error {
	reader, err := reader(filename, gzipped)
	if err != nil {
		return fmt.Errorf("error reading file %q: %s", filename, err)
	}
	defer reader.Close()

	scanner := bufio.NewScanner(reader)

	// Parse header and variables
	// --------------------------

	nVars := 0
	nClauses := 0

	for {
		if !scanner.Scan() {
			return fmt.Errorf("header line not found")
		}
		line := scanner.Text()
		if line == "" || line[0] == 'c' {
			continue
		}
		parts := strings.Fields(line)
		if parts[1] != "cnf" {
			return fmt.Errorf("instance of type %q are not supported", parts[1])
		}
		nVars, err = strconv.Atoi(parts[2])
		if err != nil {
			return fmt.Errorf("could not parse header: %w", err)
		}
		nClauses, err = strconv.Atoi(parts[3])
		if err != nil {
			return fmt.Errorf("could not parse header: %w", err)
		}

		break
	}

	for range nVars {
		dw.AddVariable()
	}

	// Parse clauses
	// -------------

	litBuffer := make([]sat.Literal, 32)
	for nClauses > 0 && scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == 'c' {
			continue
		}

		litBuffer = litBuffer[:0] // reset
		parts := strings.Fields(line)
		for _, p := range parts {
			l, err := strconv.Atoi(p)
			if err != nil {
				return err
			}
			switch {
			case l < 0:
				litBuffer = append(litBuffer, sat.NegativeLiteral(-l-1))
			case l > 0:
				litBuffer = append(litBuffer, sat.PositiveLiteral(l-1))
			default:
				// drop 0
			}
		}

		dw.AddClause(litBuffer)
		nClauses--
	}

	return nil
}
