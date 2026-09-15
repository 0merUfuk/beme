// Package benchmark is the reproducible performance harness for NFR-008.
//
// It materializes the public synthetic seed corpus
// (testdata/synthetic/records.json) as a registered source in a disposable
// deployment, optionally replicated to larger scales, and measures projection
// build time and warm-index resolution latency through the same runtime path
// the CLI and MCP server use. No private data and no network are involved.
//
// NFR-008 target: interactive resolution sub-second on the seed corpus after
// a warm index. Correctness gates outrank this target; the report records
// whether each scale met it rather than failing the build by default.
package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
)

// DefaultTasks is the fixed resolution workload (covers principle,
// precedent, preference, governance, and heuristic seed records).
var DefaultTasks = []string{
	"Decide whether this implementation task can be reported as complete",
	"Choose the storage engine for a single-writer local batch tool",
	"Pick components for a new internal service",
	"Should agent feedback update the canonical profile automatically",
	"Add a caching layer to speed up the build",
}

// Config controls one benchmark run.
type Config struct {
	SeedPath   string        // path to the public seed records JSON
	WorkDir    string        // disposable deployment root (created if missing)
	Scales     []int         // replication factors; 1 = the seed corpus as-is
	Iterations int           // timed passes over all tasks per scale
	Warmup     int           // untimed passes before measuring
	BuildRuns  int           // timed projection builds per scale
	Target     time.Duration // NFR-008 warm-resolution p95 target
	Tasks      []string
}

// Stats summarizes a latency sample in milliseconds.
type Stats struct {
	N      int     `json:"n"`
	MinMs  float64 `json:"min_ms"`
	MeanMs float64 `json:"mean_ms"`
	P50Ms  float64 `json:"p50_ms"`
	P95Ms  float64 `json:"p95_ms"`
	P99Ms  float64 `json:"p99_ms"`
	MaxMs  float64 `json:"max_ms"`
}

// ScaleResult is the measurement for one corpus scale.
type ScaleResult struct {
	Scale        int     `json:"scale"`
	Records      int     `json:"records"`
	Build        Stats   `json:"build"`
	Resolve      Stats   `json:"resolve_warm"`
	MeanItems    float64 `json:"mean_selected_items"`
	WithinTarget bool    `json:"within_target"`
}

// Report is the reproducible benchmark evidence document.
type Report struct {
	SchemaVersion string        `json:"schema_version"`
	GeneratedAt   string        `json:"generated_at"`
	GoVersion     string        `json:"go_version"`
	GOOS          string        `json:"goos"`
	GOARCH        string        `json:"goarch"`
	NumCPU        int           `json:"num_cpu"`
	SeedDigest    string        `json:"seed_digest"`
	SeedRecords   int           `json:"seed_records"`
	Iterations    int           `json:"iterations"`
	Warmup        int           `json:"warmup"`
	BuildRuns     int           `json:"build_runs"`
	Tasks         []string      `json:"tasks"`
	TargetP95Ms   float64       `json:"target_p95_ms"`
	Scales        []ScaleResult `json:"scales"`
	WithinTarget  bool          `json:"within_target"`
}

type seedRecord struct {
	SourceRecordID string `json:"source_record_id"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	Title          string `json:"title"`
	Statement      string `json:"statement"`
}

// Run executes the benchmark.
func Run(cfg Config) (*Report, error) {
	if cfg.Iterations <= 0 || cfg.BuildRuns <= 0 || len(cfg.Scales) == 0 {
		return nil, fmt.Errorf("benchmark: iterations, build runs, and scales must be positive")
	}
	if cfg.Target <= 0 {
		cfg.Target = time.Second
	}
	if len(cfg.Tasks) == 0 {
		cfg.Tasks = DefaultTasks
	}
	raw, err := os.ReadFile(cfg.SeedPath)
	if err != nil {
		return nil, fmt.Errorf("seed corpus: %w", err)
	}
	var seed []seedRecord
	if err := json.Unmarshal(raw, &seed); err != nil {
		return nil, fmt.Errorf("seed corpus: %w", err)
	}
	if len(seed) == 0 {
		return nil, fmt.Errorf("seed corpus is empty")
	}
	sum := sha256.Sum256(raw)

	rep := &Report{
		SchemaVersion: "1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		GoVersion:     runtime.Version(),
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		NumCPU:        runtime.NumCPU(),
		SeedDigest:    "sha256:" + hex.EncodeToString(sum[:]),
		SeedRecords:   len(seed),
		Iterations:    cfg.Iterations,
		Warmup:        cfg.Warmup,
		BuildRuns:     cfg.BuildRuns,
		Tasks:         cfg.Tasks,
		TargetP95Ms:   ms(cfg.Target),
		WithinTarget:  true,
	}
	for _, scale := range cfg.Scales {
		if scale <= 0 {
			return nil, fmt.Errorf("scale must be positive, got %d", scale)
		}
		res, err := runScale(cfg, seed, scale)
		if err != nil {
			return nil, fmt.Errorf("scale %d: %w", scale, err)
		}
		rep.Scales = append(rep.Scales, *res)
		rep.WithinTarget = rep.WithinTarget && res.WithinTarget
	}
	return rep, nil
}

func runScale(cfg Config, seed []seedRecord, scale int) (*ScaleResult, error) {
	root := filepath.Join(cfg.WorkDir, fmt.Sprintf("scale-%d", scale))
	rt, err := materialize(root, seed, scale)
	if err != nil {
		return nil, err
	}
	res := &ScaleResult{Scale: scale}

	builds := []time.Duration{}
	for i := 0; i < cfg.BuildRuns; i++ {
		start := time.Now()
		br, err := rt.BuildProfile(contracts.ProfileWorkSafe)
		if err != nil {
			return nil, err
		}
		builds = append(builds, time.Since(start))
		res.Records = br.RecordsIngested
	}
	if res.Records != len(seed)*scale {
		return nil, fmt.Errorf("expected %d records ingested, got %d", len(seed)*scale, res.Records)
	}
	res.Build = summarize(builds)

	sess, err := rt.Serve(contracts.ProfileWorkSafe, "cap_benchmark", false)
	if err != nil {
		return nil, err
	}
	defer sess.Store.Close()
	for i := 0; i < cfg.Warmup; i++ {
		for _, task := range cfg.Tasks {
			if _, err := sess.ResolveOnly(task, ""); err != nil {
				return nil, err
			}
		}
	}
	samples := []time.Duration{}
	items := 0
	for i := 0; i < cfg.Iterations; i++ {
		for _, task := range cfg.Tasks {
			start := time.Now()
			pack, err := sess.ResolveOnly(task, "")
			if err != nil {
				return nil, err
			}
			samples = append(samples, time.Since(start))
			items += len(pack.Constraints) + len(pack.Guidance) + len(pack.Precedents) + len(pack.Knowledge)
		}
	}
	res.Resolve = summarize(samples)
	res.MeanItems = float64(items) / float64(len(samples))
	res.WithinTarget = res.Resolve.P95Ms < ms(cfg.Target)
	return res, nil
}

// materialize writes the seed corpus (replicated scale times) as markdown
// entries plus a registered safe_declassified source, and loads a runtime.
func materialize(root string, seed []seedRecord, scale int) (*app.Runtime, error) {
	if err := os.RemoveAll(root); err != nil {
		return nil, err
	}
	cfg := filepath.Join(root, "cfg")
	entries := filepath.Join(root, "seed", "entries")
	for _, d := range []string{filepath.Join(cfg, "sources"), entries} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	for r := 0; r < scale; r++ {
		for _, rec := range seed {
			id := rec.SourceRecordID
			if r > 0 {
				id = fmt.Sprintf("%s-R%04d", id, r)
			}
			doc := fmt.Sprintf("---\nid: %s\ntitle: %q\ntype: %s\nstatus: %s\n---\n\n%s\n",
				id, rec.Title, rec.Kind, rec.Status, rec.Statement)
			if err := os.WriteFile(filepath.Join(entries, id+".md"), []byte(doc), 0o600); err != nil {
				return nil, err
			}
		}
	}
	desc := strings.Join([]string{
		`schema_version: "1"`,
		"source_id: seed-synthetic",
		"type: directory",
		"root: " + filepath.ToSlash(filepath.Join(root, "seed")),
		"purpose: [safe_declassified]",
		"trust: canonical",
		"instruction_semantics: registered_files_only",
		"authority_ceiling: recommended",
		"sensitivity: public_general",
		"profiles_allowed: [personal, work-safe]",
		"ingestion_mode: index_content",
		`include: ["entries/**/*.md"]`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(cfg, "sources", "seed.yaml"), []byte(desc), 0o600); err != nil {
		return nil, err
	}
	return app.Load(cfg)
}

func summarize(d []time.Duration) Stats {
	if len(d) == 0 {
		return Stats{}
	}
	sorted := append([]time.Duration(nil), d...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var total time.Duration
	for _, v := range sorted {
		total += v
	}
	return Stats{
		N:      len(sorted),
		MinMs:  ms(sorted[0]),
		MeanMs: round(ms(total) / float64(len(sorted))),
		P50Ms:  ms(percentile(sorted, 50)),
		P95Ms:  ms(percentile(sorted, 95)),
		P99Ms:  ms(percentile(sorted, 99)),
		MaxMs:  ms(sorted[len(sorted)-1]),
	}
}

// percentile uses the nearest-rank method on a sorted sample.
func percentile(sorted []time.Duration, p float64) time.Duration {
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

func ms(d time.Duration) float64 { return round(float64(d) / float64(time.Millisecond)) }

func round(v float64) float64 { return math.Round(v*1000) / 1000 }
