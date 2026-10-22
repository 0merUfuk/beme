package benchmark_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/0merUfuk/beme/internal/benchmark"
	"github.com/0merUfuk/beme/internal/ingestion"
)

// rec is a synthetic seed record in the public records.json shape.
type rec struct {
	SourceRecordID string `json:"source_record_id"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	Title          string `json:"title"`
	Statement      string `json:"statement"`
}

func benign(id string) rec {
	return rec{SourceRecordID: id, Kind: "principle", Status: "active", Title: "Synthetic benign record", Statement: "Synthetic statement used only by benchmark tests."}
}

// writeSeed writes records to a seed file in its own temp dir, so the sandbox
// used as the work-dir parent contains nothing the test did not expect.
func writeSeed(t *testing.T, records ...rec) string {
	t.Helper()
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "seed.json")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func runSeed(seed, work string, scales ...int) (*benchmark.Report, error) {
	if len(scales) == 0 {
		scales = []int{1}
	}
	return benchmark.Run(benchmark.Config{SeedPath: seed, WorkDir: work, Scales: scales, Iterations: 1, BuildRuns: 1})
}

// treeEntries lists every path under root (relative, slash-separated).
func treeEntries(t *testing.T, root string) []string {
	t.Helper()
	out := []string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// assertRejectedBeforeWriting: the run failed with an error naming the seed
// field, and the sandbox (parent of the work dir) is still completely empty —
// no scale dir, no entry, no escaped file anywhere.
func assertRejectedBeforeWriting(t *testing.T, name string, err error, sandbox, field string) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: unsafe seed was accepted", name)
	} else if !strings.Contains(err.Error(), field) {
		t.Errorf("%s: error does not name %s: %v", name, field, err)
	}
	if got := treeEntries(t, sandbox); len(got) != 0 {
		t.Errorf("%s: files were written before rejection: %v", name, got)
	}
}

// TestSeedIdentifiersAreConfinedToEntries pins finding 1 (path escape): every
// seed-controlled identifier that could leave the entries directory, collide
// on a case-insensitive filesystem, or name a Windows device is rejected
// before anything is written. The benign record placed first proves the
// malicious record sits on the same code path; the positive control proves the
// benign record alone is materialized and ingested.
func TestSeedIdentifiersAreConfinedToEntries(t *testing.T) {
	unsafe := map[string]string{
		"empty":               "",
		"dot":                 ".",
		"dotdot":              "..",
		"parent relative":     "../x",
		"deep traversal":      "../../../../escape",
		"slash separator":     "a/b",
		"backslash separator": `a\b`,
		"drive relative":      "C:x",
		"drive absolute":      `C:\x`,
		"unix absolute":       "/abs/x",
		"unc path":            `\\server\share\x`,
		"newline":             "SYN\n1",
		"frontmatter via id":  "SYN-1\ntype: fact",
		"nul":                 "SYN\x001",
		"tab":                 "SYN\t1",
		"leading dash":        "-SYN",
		"leading dot":         ".hidden",
		"extension dot":       "SYN.1",
		"space":               "SYN 1",
		"non-ascii":           "SYNé",
		"overlong":            strings.Repeat("A", 65),
		"windows device":      "CON",
		"windows device case": "nul",
		"windows com port":    "com1",
	}
	for name, id := range unsafe {
		sandbox := t.TempDir()
		_, err := runSeed(writeSeed(t, benign("SYN-OK"), benign(id)), filepath.Join(sandbox, "work"))
		assertRejectedBeforeWriting(t, name, err, sandbox, "source_record_id")
	}

	// Positive control: the same benign record (and the longest allowed ID)
	// on the same path is materialized inside entries and ingested.
	sandbox := t.TempDir()
	longest := strings.Repeat("A", 64)
	rep, err := runSeed(writeSeed(t, benign("SYN-OK"), benign("syn_ok_2"), benign(longest)), filepath.Join(sandbox, "work"))
	if err != nil {
		t.Fatalf("benign seed rejected: %v", err)
	}
	if rep.Scales[0].Records != 3 {
		t.Fatalf("benign seed: ingested %d records, want 3", rep.Scales[0].Records)
	}
	for _, id := range []string{"SYN-OK", "syn_ok_2", longest} {
		if _, err := os.Lstat(filepath.Join(sandbox, "work", "scale-1", "seed", "entries", id+".md")); err != nil {
			t.Fatalf("benign entry %s not materialized inside entries: %v", id, err)
		}
	}
}

// TestSeedIdentifierCollisionsAreRejected pins the filename-collision half of
// finding 1: IDs equal under case folding (macOS/Windows filesystems, and the
// ingestion record_id) and IDs that equal another record's replica name at
// the requested scale are rejected before anything is written.
func TestSeedIdentifierCollisionsAreRejected(t *testing.T) {
	cases := []struct {
		name   string
		ids    []string
		scales []int
	}{
		{"exact duplicate", []string{"SYN-1", "SYN-1"}, []int{1}},
		{"case-insensitive duplicate", []string{"SYN-1", "syn-1"}, []int{1}},
		{"replica suffix collision", []string{"SYN-1", "SYN-1-R0001"}, []int{1, 2}},
		{"replica suffix case collision", []string{"SYN-1", "syn-1-r0001"}, []int{2}},
	}
	for _, c := range cases {
		sandbox := t.TempDir()
		records := []rec{}
		for _, id := range c.ids {
			records = append(records, benign(id))
		}
		_, err := runSeed(writeSeed(t, records...), filepath.Join(sandbox, "work"), c.scales...)
		assertRejectedBeforeWriting(t, c.name, err, sandbox, "collide")
	}

	// Positive controls: a replica-looking ID is fine when the scale never
	// produces that replica, and near-miss suffixes never collide.
	for _, c := range []struct {
		ids    []string
		scales []int
		want   int
	}{
		{[]string{"SYN-1", "SYN-1-R0001"}, []int{1}, 2},
		{[]string{"SYN-1", "SYN-1-R1"}, []int{2}, 4},
	} {
		records := []rec{}
		for _, id := range c.ids {
			records = append(records, benign(id))
		}
		rep, err := runSeed(writeSeed(t, records...), filepath.Join(t.TempDir(), "work"), c.scales...)
		if err != nil {
			t.Fatalf("non-colliding ids %v at scales %v rejected: %v", c.ids, c.scales, err)
		}
		if got := rep.Scales[len(rep.Scales)-1].Records; got != c.want {
			t.Fatalf("ids %v: ingested %d records, want %d", c.ids, got, c.want)
		}
	}
}

// TestSeedMetadataCannotInjectFrontmatter pins the metadata half of finding
// 1: kind/status outside the closed enums, and title/statement values that
// would add, end, or silently alter frontmatter, are rejected before writing.
func TestSeedMetadataCannotInjectFrontmatter(t *testing.T) {
	type mut struct {
		field string
		apply func(*rec)
	}
	set := func(field, v string) mut {
		return mut{field, func(r *rec) {
			switch field {
			case "kind":
				r.Kind = v
			case "status":
				r.Status = v
			case "title":
				r.Title = v
			case "statement":
				r.Statement = v
			}
		}}
	}
	unsafe := map[string]mut{
		"kind newline injects status":   set("kind", "principle\nstatus: deprecated"),
		"kind closes frontmatter":       set("kind", "principle\n---\ninjected"),
		"kind unknown":                  set("kind", "bogus"),
		"kind wrong case":               set("kind", "Principle"),
		"kind padded":                   set("kind", " principle"),
		"kind empty":                    set("kind", ""),
		"status newline injects kind":   set("status", "active\ntype: fact"),
		"status comment":                set("status", "active # note"),
		"status unknown":                set("status", "archived"),
		"status deprecated not indexed": set("status", "deprecated"),
		"title newline injects status":  set("title", "Harmless\nstatus: deprecated"),
		"title closes frontmatter":      set("title", "Harmless\n---\ninjected: true"),
		"title escaped quote":           set("title", `Harmless" status: "deprecated`),
		"title backslash":               set("title", `C:\path`),
		"title comment truncation":      set("title", "Rule #1 matters"),
		"title leading quote":           set("title", "'quoted'"),
		"title padded":                  set("title", " padded "),
		"title tab":                     set("title", "tab\there"),
		"title line separator":          set("title", "line\u2028separator"),
		"title bidi override":           set("title", "abc\u202edef"),
		"title nul":                     set("title", "nul\x00byte"),
		"title empty":                   set("title", ""),
		"title overlong":                set("title", strings.Repeat("t", 201)),
		"statement newline":             set("statement", "first\n---\nid: X"),
		"statement markdown heading":    set("statement", "# not the statement"),
		"statement empty":               set("statement", ""),
		"statement overlong":            set("statement", strings.Repeat("s", 601)),
	}
	for name, m := range unsafe {
		sandbox := t.TempDir()
		bad := benign("SYN-BAD")
		m.apply(&bad)
		_, err := runSeed(writeSeed(t, benign("SYN-OK"), bad), filepath.Join(sandbox, "work"))
		assertRejectedBeforeWriting(t, name, err, sandbox, m.field)
	}

	// Positive control: YAML-significant but representable values are
	// accepted, ingested, and round-trip exactly through both the ingestion
	// frontmatter parser and a real YAML parser.
	safe := rec{
		SourceRecordID: "true",
		Kind:           "failure_mode",
		Status:         "active",
		Title:          "Colon: yes, [brackets] {braces} & *star* !bang %pct @at `tick` it's 100% | > - ? " + strings.Repeat("long ", 20) + "end", // > 80 columns: no line wrapping
		Statement:      "A statement with: colons, 'quotes', \"double\", #hash, and --- dashes.",
	}
	work := filepath.Join(t.TempDir(), "work")
	rep, err := runSeed(writeSeed(t, benign("SYN-OK"), safe), work)
	if err != nil {
		t.Fatalf("representable seed rejected: %v", err)
	}
	if rep.Scales[0].Records != 2 {
		t.Fatalf("representable seed: ingested %d records, want 2", rep.Scales[0].Records)
	}
	doc, err := os.ReadFile(filepath.Join(work, "scale-1", "seed", "entries", "true.md"))
	if err != nil {
		t.Fatal(err)
	}
	fm, body := ingestion.ParseMarkdown(doc)
	want := map[string]any{"id": safe.SourceRecordID, "title": safe.Title, "type": safe.Kind, "status": safe.Status}
	if len(fm) != len(want) {
		t.Fatalf("ingestion frontmatter keys = %v, want exactly %v", fm, want)
	}
	for k, v := range want {
		if fm[k] != v {
			t.Fatalf("ingestion frontmatter %s = %q, want %q", k, fm[k], v)
		}
	}
	if strings.TrimSpace(body) != safe.Statement {
		t.Fatalf("body = %q, want %q", body, safe.Statement)
	}
	parts := strings.SplitN(string(doc), "\n---\n", 2)
	var yfm map[string]any
	if err := yaml.Unmarshal([]byte(strings.TrimPrefix(parts[0], "---\n")), &yfm); err != nil {
		t.Fatalf("frontmatter is not valid YAML: %v", err)
	}
	if len(yfm) != len(want) {
		t.Fatalf("YAML frontmatter keys = %v, want exactly %v", yfm, want)
	}
	for k, v := range want {
		if yfm[k] != v {
			t.Fatalf("YAML frontmatter %s = %#v, want %#v", k, yfm[k], v)
		}
	}
}

// TestMaterializeNeverFollowsPlantedSymlinks pins finding 2: symlinks planted
// inside the work dir are never followed for deletion or writing. A symlinked
// scale dir is refused; symlinked children of a harness-owned scale dir are
// unlinked, never written through, and replaced by real directories.
func TestMaterializeNeverFollowsPlantedSymlinks(t *testing.T) {
	seed := writeSeed(t, benign("SYN-OK"))
	sandbox := t.TempDir()

	// (a) scale-1 is a symlink to a directory that even carries the marker.
	outsideA := filepath.Join(sandbox, "outside-a")
	if err := os.MkdirAll(outsideA, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{".beme-bench-scale", "user-data.txt"} {
		if err := os.WriteFile(filepath.Join(outsideA, f), []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	workA := filepath.Join(sandbox, "work-a")
	if err := os.MkdirAll(workA, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideA, filepath.Join(workA, "scale-1")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := runSeed(seed, workA); err == nil {
		t.Error("a symlinked scale directory must be refused")
	}
	if got := treeEntries(t, outsideA); strings.Join(got, ",") != ".beme-bench-scale,user-data.txt" {
		t.Errorf("symlinked scale dir target was modified: %v", got)
	}

	// (b) a harness-owned scale-1 whose seed/entries and cfg are symlinks.
	outsideB := filepath.Join(sandbox, "outside-b")
	outsideC := filepath.Join(sandbox, "outside-c")
	for _, d := range []string{outsideB, outsideC} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "user-data.txt"), []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	workB := filepath.Join(sandbox, "work-b")
	owned := filepath.Join(workB, "scale-1")
	if err := os.MkdirAll(filepath.Join(owned, "seed"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(owned, ".beme-bench-scale"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideB, filepath.Join(owned, "seed", "entries")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideC, filepath.Join(owned, "cfg")); err != nil {
		t.Fatal(err)
	}
	rep, err := runSeed(seed, workB)
	// Check the link targets first: a write through a link must be reported
	// even when it also breaks ingestion.
	for _, d := range []string{outsideB, outsideC} {
		if got := treeEntries(t, d); strings.Join(got, ",") != "user-data.txt" {
			t.Errorf("symlink target %s was written through or deleted: %v", d, got)
		}
	}
	if err != nil {
		t.Fatalf("harness-owned scale dir with planted symlinks: %v", err)
	}
	if rep.Scales[0].Records != 1 {
		t.Fatalf("ingested %d records, want 1", rep.Scales[0].Records)
	}
	for _, d := range []string{filepath.Join(owned, "seed", "entries"), filepath.Join(owned, "cfg")} {
		info, err := os.Lstat(d)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			t.Errorf("%s must be a real directory after materialize (info=%v err=%v)", d, info, err)
		}
	}
}
