package benchmark_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/benchmark"
)

func seedPath() string {
	return filepath.Join("..", "..", "testdata", "synthetic", "records.json")
}

// TestBenchmarkHarnessOnSeedCorpus proves the harness mechanics on the public
// seed corpus: every scale ingests exactly seed×scale records, every timed
// sample is counted, resolution selects real items, and the report is
// serializable evidence. Timings are asserted structurally only.
func TestBenchmarkHarnessOnSeedCorpus(t *testing.T) {
	rep, err := benchmark.Run(benchmark.Config{
		SeedPath:   seedPath(),
		WorkDir:    t.TempDir(),
		Scales:     []int{1, 3},
		Iterations: 2,
		Warmup:     1,
		BuildRuns:  1,
		Target:     time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.SeedRecords != 5 || len(rep.Scales) != 2 {
		t.Fatalf("unexpected report shape: seed=%d scales=%d", rep.SeedRecords, len(rep.Scales))
	}
	for _, s := range rep.Scales {
		if s.Records != rep.SeedRecords*s.Scale {
			t.Fatalf("scale %d: records %d, want %d", s.Scale, s.Records, rep.SeedRecords*s.Scale)
		}
		if s.Resolve.N != rep.Iterations*len(rep.Tasks) || s.Build.N != rep.BuildRuns {
			t.Fatalf("scale %d: sample counts wrong: %+v", s.Scale, s)
		}
		if s.Resolve.P50Ms > s.Resolve.P95Ms || s.Resolve.P95Ms > s.Resolve.MaxMs {
			t.Fatalf("scale %d: percentiles not monotonic: %+v", s.Scale, s.Resolve)
		}
		if s.MeanItems <= 0 {
			t.Fatalf("scale %d: resolution selected nothing — benchmark would measure empty packs", s.Scale)
		}
	}
	if _, err := json.Marshal(rep); err != nil {
		t.Fatal(err)
	}
}

func TestBenchmarkRejectsInvalidConfig(t *testing.T) {
	if _, err := benchmark.Run(benchmark.Config{SeedPath: seedPath(), WorkDir: t.TempDir(), Scales: []int{1}}); err == nil {
		t.Fatal("zero iterations must be rejected")
	}
	if _, err := benchmark.Run(benchmark.Config{SeedPath: "missing.json", WorkDir: t.TempDir(), Scales: []int{1}, Iterations: 1, BuildRuns: 1}); err == nil {
		t.Fatal("missing seed corpus must be rejected")
	}
}

// BenchmarkResolveWarmSeed exposes warm resolution to `go test -bench`.
func BenchmarkResolveWarmSeed(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := benchmark.Run(benchmark.Config{
			SeedPath: seedPath(), WorkDir: b.TempDir(), Scales: []int{1}, Iterations: 1, BuildRuns: 1,
		}); err != nil {
			b.Fatal(err)
		}
	}
}
