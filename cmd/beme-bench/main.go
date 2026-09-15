// Command beme-bench runs the NFR-008 performance benchmark on the public
// synthetic seed corpus and writes a JSON evidence report.
//
//	go run ./cmd/beme-bench --seed testdata/synthetic/records.json --out -
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/benchmark"
)

func main() {
	seed := flag.String("seed", "testdata/synthetic/records.json", "public seed corpus (normalized records JSON)")
	scales := flag.String("scales", "1,20,200", "comma-separated replication factors (1 = seed corpus as-is)")
	iterations := flag.Int("iterations", 50, "timed passes over the task set per scale")
	warmup := flag.Int("warmup", 5, "untimed warm-up passes per scale")
	buildRuns := flag.Int("build-runs", 3, "timed projection builds per scale")
	target := flag.Duration("target", time.Second, "NFR-008 warm-resolution p95 target")
	out := flag.String("out", "-", "report path ('-' for stdout)")
	work := flag.String("work", "", "deployment work dir (default: a temp dir removed afterwards)")
	enforce := flag.Bool("enforce", false, "exit 1 when any scale misses the target")
	flag.Parse()

	factors := []int{}
	for _, f := range strings.Split(*scales, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n <= 0 {
			fmt.Fprintf(os.Stderr, "error: invalid scale %q\n", f)
			os.Exit(2)
		}
		factors = append(factors, n)
	}
	dir := *work
	if dir == "" {
		tmp, err := os.MkdirTemp("", "beme-bench-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		defer os.RemoveAll(tmp)
		dir = tmp
	}

	rep, err := benchmark.Run(benchmark.Config{
		SeedPath: *seed, WorkDir: dir, Scales: factors,
		Iterations: *iterations, Warmup: *warmup, BuildRuns: *buildRuns, Target: *target,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	data, _ := json.MarshalIndent(rep, "", "  ")
	data = append(data, '\n')
	if *out == "-" {
		os.Stdout.Write(data)
	} else if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "beme-bench %s/%s %s cpus=%d target p95 < %.0fms\n", rep.GOOS, rep.GOARCH, rep.GoVersion, rep.NumCPU, rep.TargetP95Ms)
	for _, s := range rep.Scales {
		fmt.Fprintf(os.Stderr, "  scale %-4d records=%-5d build p50=%.1fms  resolve p50=%.2fms p95=%.2fms max=%.2fms  items=%.1f  within_target=%v\n",
			s.Scale, s.Records, s.Build.P50Ms, s.Resolve.P50Ms, s.Resolve.P95Ms, s.Resolve.MaxMs, s.MeanItems, s.WithinTarget)
	}
	if *enforce && !rep.WithinTarget {
		os.Exit(1)
	}
}
