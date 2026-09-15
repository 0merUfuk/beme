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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/ingestion"
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

// Run executes the benchmark. The seed corpus is fully validated — including
// every replica entry at the largest requested scale — before anything is
// written under the work dir.
func Run(cfg Config) (*Report, error) {
	if cfg.Iterations <= 0 || cfg.BuildRuns <= 0 || len(cfg.Scales) == 0 {
		return nil, fmt.Errorf("benchmark: iterations, build runs, and scales must be positive")
	}
	maxScale := 0
	for _, scale := range cfg.Scales {
		if scale <= 0 {
			return nil, fmt.Errorf("scale must be positive, got %d", scale)
		}
		maxScale = max(maxScale, scale)
	}
	if err := validateWorkDir(cfg.WorkDir); err != nil {
		return nil, err
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
	if err := validateSeed(seed, maxScale); err != nil {
		return nil, err
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
// The seed must already have passed validateSeed. Every directory under root
// is freshly created (never reused) and confirmed with Lstat, and every file
// is created exclusively, so nothing planted under the work dir is followed.
func materialize(root string, seed []seedRecord, scale int) (*app.Runtime, error) {
	if err := removeOwnedScaleDir(root); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o700); err != nil {
		return nil, err
	}
	if _, err := mkdirFresh(filepath.Dir(root), filepath.Base(root)); err != nil {
		return nil, err
	}
	// Mark ownership first so a partially materialized dir stays reclaimable.
	if err := writeNewFile(filepath.Join(root, scaleMarker), []byte("created by beme-bench; safe to delete\n")); err != nil {
		return nil, err
	}
	cfg, err := mkdirFresh(root, "cfg")
	if err != nil {
		return nil, err
	}
	sources, err := mkdirFresh(cfg, "sources")
	if err != nil {
		return nil, err
	}
	seedDir, err := mkdirFresh(root, "seed")
	if err != nil {
		return nil, err
	}
	entries, err := mkdirFresh(seedDir, "entries")
	if err != nil {
		return nil, err
	}
	for r := 0; r < scale; r++ {
		for _, rec := range seed {
			id := replicaID(rec.SourceRecordID, r)
			if err := writeNewFile(filepath.Join(entries, id+".md"), renderEntry(id, rec)); err != nil {
				return nil, err
			}
		}
	}
	desc := strings.Join([]string{
		`schema_version: "1"`,
		"source_id: seed-synthetic",
		"type: directory",
		"root: " + filepath.ToSlash(seedDir),
		"purpose: [safe_declassified]",
		"trust: canonical",
		"instruction_semantics: registered_files_only",
		"authority_ceiling: recommended",
		"sensitivity: public_general",
		"profiles_allowed: [personal, work-safe]",
		"ingestion_mode: index_content",
		`include: ["entries/**/*.md"]`,
	}, "\n") + "\n"
	if err := writeNewFile(filepath.Join(sources, "seed.yaml"), []byte(desc)); err != nil {
		return nil, err
	}
	return app.Load(cfg)
}

// mkdirFresh creates parent/name, failing if anything (including a symlink)
// already exists there, and confirms without following links that the result
// is a real directory.
func mkdirFresh(parent, name string) (string, error) {
	p := filepath.Join(parent, name)
	if err := os.Mkdir(p, 0o700); err != nil {
		return "", fmt.Errorf("benchmark: create %s: %w", p, err)
	}
	info, err := os.Lstat(p)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("benchmark: refusing %s: not a real directory", p)
	}
	return p, nil
}

// writeNewFile creates path exclusively: an existing file, a symlink (even a
// dangling one), or a case-insensitive name collision is an error, never an
// overwrite or a write through a link.
func writeNewFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("benchmark: create %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("benchmark: write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("benchmark: write %s: %w", path, err)
	}
	return nil
}

// Seed field limits. Statements longer than the ingestion statement limit
// would be truncated on ingest, so they are rejected rather than altered.
const (
	maxSeedIDBytes        = 64
	maxSeedTitleBytes     = 200
	maxSeedStatementBytes = 600
)

func replicaID(base string, r int) string {
	if r == 0 {
		return base
	}
	return fmt.Sprintf("%s-R%04d", base, r)
}

// validateSeed rejects any seed whose materialization could leave the entries
// directory, collide with another entry, or alter entry metadata. It checks
// each record's fields, then renders every entry that materialize will write
// at maxScale and proves it round-trips exactly.
func validateSeed(seed []seedRecord, maxScale int) error {
	fail := func(i int, err error) error {
		return fmt.Errorf("seed corpus: record %d (%q): %w", i, seed[i].SourceRecordID, err)
	}
	for i, rec := range seed {
		if err := validateSeedRecord(rec); err != nil {
			return fail(i, err)
		}
	}
	type owner struct{ record, replica int }
	names := map[string]owner{}     // case-folded entry file name
	recordIDs := map[string]owner{} // normalized ingestion record_id
	for r := 0; r < maxScale; r++ {
		for i, rec := range seed {
			id := replicaID(rec.SourceRecordID, r)
			recordID, err := verifyEntry(renderEntry(id, rec), id, rec)
			if err != nil {
				return fail(i, err)
			}
			if prev, dup := names[strings.ToLower(id)]; dup {
				return fail(i, fmt.Errorf("entry %q (replica %d) would collide case-insensitively with record %d replica %d", id, r, prev.record, prev.replica))
			}
			if prev, dup := recordIDs[recordID]; dup {
				return fail(i, fmt.Errorf("entry %q (replica %d) would collide with record %d replica %d as record_id %q", id, r, prev.record, prev.replica, recordID))
			}
			names[strings.ToLower(id)] = owner{i, r}
			recordIDs[recordID] = owner{i, r}
		}
	}
	return nil
}

func validateSeedRecord(rec seedRecord) error {
	if err := validateSeedID(rec.SourceRecordID); err != nil {
		return err
	}
	if !contracts.Kind(rec.Kind).Valid() {
		return fmt.Errorf("kind %q is not a normalized record kind", rec.Kind)
	}
	switch contracts.Status(rec.Status) {
	case contracts.StatusActive:
	case contracts.StatusDeprecated:
		return errors.New("status \"deprecated\" records are never indexed; every seed record must be active")
	default:
		return fmt.Errorf("status %q is not a canonical status", rec.Status)
	}
	if err := validateSeedText("title", rec.Title, maxSeedTitleBytes); err != nil {
		return err
	}
	// The ingestion frontmatter parser strips surrounding quotes and cuts at
	// '#'; the renderer's quoting needs no '"' or '\'.
	if strings.ContainsAny(rec.Title, "\"\\#") || strings.HasPrefix(rec.Title, "'") || strings.HasSuffix(rec.Title, "'") {
		return errors.New(`title must not contain '"', '\', or '#', or start or end with '''`)
	}
	if err := validateSeedText("statement", rec.Statement, maxSeedStatementBytes); err != nil {
		return err
	}
	if strings.ContainsAny(rec.Statement[:1], "#[!") {
		return errors.New("statement must not start with '#', '[', or '!' (markdown the entry parser skips)")
	}
	return nil
}

// validateSeedID is the strict allowlist for identifiers that become entry
// file names: [A-Za-z0-9][A-Za-z0-9_-]*, at most maxSeedIDBytes. It excludes
// '.', '/', '\', ':', whitespace, control characters, and non-ASCII (whose
// case folding and normalization differ across filesystems), so no ID can be
// a traversal, a separator, a drive or UNC prefix, or an absolute path.
func validateSeedID(id string) error {
	if id == "" {
		return errors.New("source_record_id is empty")
	}
	if len(id) > maxSeedIDBytes {
		return fmt.Errorf("source_record_id is longer than %d bytes", maxSeedIDBytes)
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || (i > 0 && (c == '_' || c == '-')) {
			continue
		}
		return errors.New("source_record_id must match [A-Za-z0-9][A-Za-z0-9_-]* (no dots, separators, drive prefixes, whitespace, control or non-ASCII characters)")
	}
	if windowsDeviceName(id) {
		return errors.New("source_record_id is a reserved Windows device name")
	}
	return nil
}

func windowsDeviceName(id string) bool {
	u := strings.ToUpper(id)
	switch u {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(u) == 4 && (strings.HasPrefix(u, "COM") || strings.HasPrefix(u, "LPT")) && u[3] >= '0' && u[3] <= '9'
}

func validateSeedText(field, v string, limit int) error {
	switch {
	case v == "":
		return fmt.Errorf("%s is empty", field)
	case len(v) > limit:
		return fmt.Errorf("%s is longer than %d bytes", field, limit)
	case !utf8.ValidString(v):
		return fmt.Errorf("%s is not valid UTF-8", field)
	case strings.TrimSpace(v) != v:
		return fmt.Errorf("%s has leading or trailing whitespace", field)
	}
	for _, r := range v {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' ' {
			return fmt.Errorf("%s contains control, format, or line-separator character %U", field, r)
		}
	}
	return nil
}

// renderEntry emits one entry document. Every frontmatter value is a YAML
// double-quoted scalar. validateSeedRecord guarantees no value contains '"',
// '\', or a line break, so each scalar's content is literal YAML (escapes need
// '\', folding needs a break) and is exactly what ingestion.ParseMarkdown
// yields after stripping the quotes. verifyEntry re-proves this per document.
func renderEntry(id string, rec seedRecord) []byte {
	return []byte("---\n" +
		"id: \"" + id + "\"\n" +
		"title: \"" + rec.Title + "\"\n" +
		"type: \"" + rec.Kind + "\"\n" +
		"status: \"" + rec.Status + "\"\n" +
		"---\n\n" + rec.Statement + "\n")
}

// verifyEntry proves a rendered entry carries exactly the intended metadata
// under both the ingestion frontmatter parser and a real YAML parser, and
// that ingestion indexes it with the intended ID and statement. It returns
// the normalized record_id.
func verifyEntry(doc []byte, id string, rec seedRecord) (string, error) {
	want := []struct{ key, field, value string }{
		{"id", "source_record_id", id},
		{"title", "title", rec.Title},
		{"type", "kind", rec.Kind},
		{"status", "status", rec.Status},
	}
	fm, body := ingestion.ParseMarkdown(doc)
	end := bytes.Index(doc, []byte("\n---\n"))
	if end < 0 {
		return "", errors.New("entry frontmatter is not terminated")
	}
	var yfm map[string]any
	if err := yaml.Unmarshal(doc[len("---\n"):end+1], &yfm); err != nil {
		return "", fmt.Errorf("entry frontmatter is not valid YAML: %w", err)
	}
	if len(fm) != len(want) || len(yfm) != len(want) {
		return "", fmt.Errorf("entry frontmatter has %d/%d keys, want exactly %d", len(fm), len(yfm), len(want))
	}
	for _, w := range want {
		if got, ok := fm[w.key].(string); !ok || got != w.value {
			return "", fmt.Errorf("%s does not round-trip through the entry frontmatter parser", w.field)
		}
		if got, ok := yfm[w.key].(string); !ok || got != w.value {
			return "", fmt.Errorf("%s does not round-trip through YAML", w.field)
		}
	}
	norm, ok := ingestion.NormalizeEntry(contracts.SourceDescriptor{SourceID: "seed-synthetic"}, ingestion.MarkdownEntry{Frontmatter: fm, Body: body})
	if !ok || norm.SourceRecordID != id {
		return "", errors.New("source_record_id would not be indexed by the entry adapter")
	}
	if norm.Title != rec.Title {
		return "", errors.New("title does not round-trip through the entry adapter")
	}
	if norm.Statement != rec.Statement {
		return "", errors.New("statement does not round-trip through the entry adapter")
	}
	return norm.RecordID, nil
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

// scaleMarker marks directories this harness created; only those are ever
// deleted.
const scaleMarker = ".beme-bench-scale"

// validateWorkDir rejects work dirs where deleting and recreating scale-N
// directories could reach user data: empty, relative, a filesystem root, the
// home directory or its ancestors, the current working directory or its
// ancestors, and repository or workspace roots.
func validateWorkDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("benchmark: work dir is required (scale directories under it are deleted and recreated)")
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("benchmark: work dir must be an absolute path, got %q", dir)
	}
	clean := filepath.Clean(dir)
	if filepath.Dir(clean) == clean {
		return fmt.Errorf("benchmark: refusing filesystem root %q as work dir", clean)
	}
	candidates := withRealPath(clean)
	covers := func(protected string) bool {
		for _, c := range candidates {
			for _, p := range withRealPath(protected) {
				if c == p || isAncestor(c, p) {
					return true
				}
			}
		}
		return false
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && covers(home) {
		return fmt.Errorf("benchmark: refusing %q: it is or contains the home directory", clean)
	}
	if wd, err := os.Getwd(); err == nil && covers(wd) {
		return fmt.Errorf("benchmark: refusing %q: it is or contains the current working directory", clean)
	}
	for _, marker := range []string{".git", ".hg", ".svn", "go.mod"} {
		if _, err := os.Lstat(filepath.Join(clean, marker)); err == nil {
			return fmt.Errorf("benchmark: refusing %q: it looks like a repository or workspace root (%s present)", clean, marker)
		}
	}
	return nil
}

func withRealPath(p string) []string {
	out := []string{filepath.Clean(p)}
	if r, err := filepath.EvalSymlinks(p); err == nil && r != out[0] {
		out = append(out, r)
	}
	return out
}

func isAncestor(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// removeOwnedScaleDir deletes a previous scale directory only when it is a
// real directory (not a symlink) carrying this harness's marker as a regular
// file; any other existing path is refused. os.RemoveAll unlinks symlinks
// found inside without following them.
func removeOwnedScaleDir(root string) error {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("benchmark: refusing to replace %s: not a directory created by beme-bench", root)
	}
	if m, err := os.Lstat(filepath.Join(root, scaleMarker)); err != nil || !m.Mode().IsRegular() {
		return fmt.Errorf("benchmark: refusing to delete %s: it was not created by beme-bench (no %s marker)", root, scaleMarker)
	}
	return os.RemoveAll(root)
}
