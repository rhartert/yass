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

var flagMaxConflict = flag.Int64(
	"max_conflicts",
	-1,
	"maximum number of conflicts allowed to solve the problem (-1 = no maximum)",
)

var flagTimeout = flag.Duration(
	"timeout",
	-1,
	"search timeout (-1 = no timeout)",
)

var flagPhaseSaving = flag.Bool(
	"phase",
	false,
	"enable phase saving in search strategy",
)

var flagGzipInput = flag.Bool(
	"gzip",
	false,
	"gzipped input DIMACS file",
)

func parseConfig() (*config, error) {
	flag.Parse()

	if flag.NArg() == 0 || flag.Arg(0) == "" {
		return nil, fmt.Errorf("missing instance file")
	}
	return &config{
		instanceFile: flag.Arg(0),
		gzippedFile:  *flagGzipInput,
		memProfile:   *flagMemProfile,
		cpuProfile:   *flagCPUProfile,
		maxConflicts: *flagMaxConflict,
		timeout:      *flagTimeout,
		phaseSaving:  *flagPhaseSaving,
	}, nil
}

type config struct {
	instanceFile string
	gzippedFile  bool
	memProfile   bool
	cpuProfile   bool
	maxConflicts int64
	timeout      time.Duration
	phaseSaving  bool
}

func solverOptions(cfg *config) sat.Options {
	options := sat.DefaultOptions
	options.PhaseSaving = cfg.phaseSaving
	if cfg.maxConflicts >= 0 {
		options.MaxConflicts = cfg.maxConflicts
	}
	if cfg.timeout >= 0 {
		options.Timeout = cfg.timeout
	}
	return options
}

func run(cfg *config) error {
	instance, err := dimacs.ParseDIMACS(cfg.instanceFile, cfg.gzippedFile)
	if err != nil {
		return fmt.Errorf("could not parse instance: %s", err)
	}

	s := sat.NewSolver(solverOptions(cfg))
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
