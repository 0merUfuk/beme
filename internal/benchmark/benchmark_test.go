package benchmark_test

import (
	"encoding/json"
	"os"
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

// BenchmarkHarnessSeedEndToEnd times one full harness run on the seed corpus:
// corpus load, disposable deployment, projection build, and a single timed
// resolution pass. It is not a warm-resolution microbenchmark — warm latency
// is what benchmark.Run reports per scale (resolve_warm).
func BenchmarkHarnessSeedEndToEnd(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := benchmark.Run(benchmark.Config{
			SeedPath: seedPath(), WorkDir: b.TempDir(), Scales: []int{1}, Iterations: 1, BuildRuns: 1,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

// TestBenchmarkRefusesUnsafeWorkDir pins the destructive-target guard: every
// unsafe work dir is rejected before anything is deleted, and a pre-existing
// directory the harness did not create is never removed.
func TestBenchmarkRefusesUnsafeWorkDir(t *testing.T) {
	seed, err := filepath.Abs(seedPath())
	if err != nil {
		t.Fatal(err)
	}
	sandbox := t.TempDir()
	home := filepath.Join(sandbox, "home")
	cwd := filepath.Join(sandbox, "work", "cwd")
	repo := filepath.Join(sandbox, "repo")
	for _, d := range []string{home, cwd, filepath.Join(repo, ".git")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sentinels := []string{}
	for _, d := range []string{home, cwd, filepath.Dir(cwd), repo} {
		p := filepath.Join(d, "scale-1", "user-data.txt")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		sentinels = append(sentinels, p)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(cwd)

	root := filepath.VolumeName(sandbox) + string(filepath.Separator)
	unsafe := map[string]string{
		"empty":           "",
		"blank":           "   ",
		"relative":        "bench-out",
		"dot":             ".",
		"filesystem root": root,
		"home":            home,
		"home ancestor":   filepath.Dir(home),
		"cwd":             cwd,
		"cwd ancestor":    filepath.Dir(cwd),
		"repository root": repo,
	}
	for name, dir := range unsafe {
		_, err := benchmark.Run(benchmark.Config{SeedPath: seed, WorkDir: dir, Scales: []int{1}, Iterations: 1, BuildRuns: 1})
		if err == nil {
			t.Errorf("%s work dir %q must be rejected", name, dir)
		}
	}

	// A safe dir holding a pre-existing scale-1 the harness did not create.
	foreign := filepath.Join(sandbox, "foreign")
	foreignFile := filepath.Join(foreign, "scale-1", "user-data.txt")
	if err := os.MkdirAll(filepath.Dir(foreignFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreignFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	sentinels = append(sentinels, foreignFile)
	if _, err := benchmark.Run(benchmark.Config{SeedPath: seed, WorkDir: foreign, Scales: []int{1}, Iterations: 1, BuildRuns: 1}); err == nil {
		t.Error("an unmarked pre-existing scale directory must not be deleted")
	}
	for _, p := range sentinels {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("guard deleted user data %s: %v", p, err)
		}
	}

	// The harness's own directories are reusable across runs.
	own := filepath.Join(sandbox, "own")
	for i := 0; i < 2; i++ {
		if _, err := benchmark.Run(benchmark.Config{SeedPath: seed, WorkDir: own, Scales: []int{1}, Iterations: 1, BuildRuns: 1}); err != nil {
			t.Fatalf("run %d in a harness-owned work dir: %v", i, err)
		}
	}
}
