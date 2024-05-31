package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"time"

	"github.com/rhartert/yass/parsers"
	"github.com/rhartert/yass/sat"
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
	s := sat.NewSolver(solverOptions(cfg))

	tRead := time.Now()
	if err := parsers.LoadDIMACS(cfg.instanceFile, cfg.gzippedFile, s); err != nil {
		return fmt.Errorf("could not load instance: %s", err)
	}

	tSolve := time.Now()
	status := s.Solve()
	tCompleted := time.Now()

	stats := s.Statistics
	readDur := tSolve.Sub(tRead).Seconds()
	solveDur := tCompleted.Sub(tSolve).Seconds()
	propagationsFreq := float64(stats.Propagations) / solveDur
	conflictsFreq := float64(stats.Conflicts) / solveDur

	fmt.Printf("c\n")
	fmt.Printf("c read time:    %.3f sec\n", readDur)
	fmt.Printf("c solve time:   %.3f sec\n", solveDur)
	fmt.Printf("c conflicts:    %d (%.2f /sec)\n", stats.Conflicts, conflictsFreq)
	fmt.Printf("c propagations: %d (%.2f M/sec)\n", stats.Propagations, propagationsFreq/1e6)
	fmt.Printf("c status:       %s\n", status.String())

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
