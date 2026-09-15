// Command beme-bench runs the NFR-008 performance benchmark on the public
// synthetic seed corpus and writes a JSON evidence report.
//
//	go run ./cmd/beme-bench --seed testdata/synthetic/records.json --out -
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/benchmark"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run returns the exit code so deferred cleanup always executes before exit.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("beme-bench", flag.ContinueOnError)
	fs.SetOutput(stderr)
	seed := fs.String("seed", "testdata/synthetic/records.json", "public seed corpus (normalized records JSON)")
	scales := fs.String("scales", "1,20,200", "comma-separated replication factors (1 = seed corpus as-is)")
	iterations := fs.Int("iterations", 50, "timed passes over the task set per scale")
	warmup := fs.Int("warmup", 5, "untimed warm-up passes per scale")
	buildRuns := fs.Int("build-runs", 3, "timed projection builds per scale")
	target := fs.Duration("target", time.Second, "NFR-008 warm-resolution p95 target")
	out := fs.String("out", "-", "report path ('-' for stdout)")
	work := fs.String("work", "", "absolute deployment work dir (default: a temp dir removed afterwards)")
	enforce := fs.Bool("enforce", false, "exit 1 when any scale misses the target")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	factors := []int{}
	for _, f := range strings.Split(*scales, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n <= 0 {
			fmt.Fprintf(stderr, "error: invalid scale %q\n", f)
			return 2
		}
		factors = append(factors, n)
	}
	dir := *work
	if dir == "" {
		tmp, err := os.MkdirTemp("", "beme-bench-")
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		defer os.RemoveAll(tmp)
		dir = tmp
	}

	rep, err := benchmark.Run(benchmark.Config{
		SeedPath: *seed, WorkDir: dir, Scales: factors,
		Iterations: *iterations, Warmup: *warmup, BuildRuns: *buildRuns, Target: *target,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	data = append(data, '\n')
	if *out == "-" {
		if _, err := stdout.Write(data); err != nil {
			fmt.Fprintf(stderr, "error: write report: %v\n", err)
			return 1
		}
	} else if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(stderr, "error: write report: %v\n", err)
		return 1
	}

	fmt.Fprintf(stderr, "beme-bench %s/%s %s cpus=%d target p95 < %.0fms\n", rep.GOOS, rep.GOARCH, rep.GoVersion, rep.NumCPU, rep.TargetP95Ms)
	for _, s := range rep.Scales {
		fmt.Fprintf(stderr, "  scale %-4d records=%-5d build p50=%.1fms  resolve p50=%.2fms p95=%.2fms max=%.2fms  items=%.1f  within_target=%v\n",
			s.Scale, s.Records, s.Build.P50Ms, s.Resolve.P50Ms, s.Resolve.P95Ms, s.Resolve.MaxMs, s.MeanItems, s.WithinTarget)
	}
	if *enforce && !rep.WithinTarget {
		return 1
	}
	return 0
}
