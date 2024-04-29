package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"time"

	"github.com/rhartert/yass/internal/dimacs"
	"github.com/rhartert/yass/internal/sat"
)

var flagCPUProfile = flag.Bool(
	"cpuprof",
	false,
	"save pprof CPU profile in cpuprof",
)

var flagMemProfile = flag.Bool(
	"memprof",
	false,
	"save pprof memory profile in memprof",
)

func parseConfig() (*config, error) {
	flag.Parse()

	if flag.NArg() == 0 || flag.Arg(0) == "" {
		return nil, fmt.Errorf("missing instance file")
	}
	return &config{
		instanceFile: flag.Arg(0),
		memProfile:   *flagMemProfile,
		cpuProfile:   *flagCPUProfile,
	}, nil
}

type config struct {
	instanceFile string
	memProfile   bool
	cpuProfile   bool
}

func run(cfg *config) error {
	instance, err := dimacs.ParseDIMACS(cfg.instanceFile)
	if err != nil {
		return fmt.Errorf("could not parse instance: %s", err)
	}

	s := sat.NewDefaultSolver()
	dimacs.Instantiate(s, instance)

	fmt.Printf("c variables:  %d\n", instance.Variables)
	fmt.Printf("c clauses:    %d\n", len(instance.Clauses))

	t := time.Now()
	status := s.Solve()
	elapsed := time.Since(t)

	fmt.Printf("c time (sec): %f\n", elapsed.Seconds())
	fmt.Printf("c conflicts:  %d (%.2f /sec)\n", s.TotalConflicts, float64(s.TotalConflicts)/elapsed.Seconds())
	fmt.Printf("c status:     %s\n", status.String())

	return nil
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		log.Fatal(err)
	}

	if cfg.cpuProfile {
		f, err := os.Create("cpuprof")
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if err := run(cfg); err != nil {
		log.Fatal(err)
	}

	if cfg.memProfile {
		f, err := os.Create("memprof")
		if err != nil {
			log.Fatal(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
		return
	}
}
