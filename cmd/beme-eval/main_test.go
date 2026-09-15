package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/evalrunner"
	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
	"github.com/0merUfuk/beme/internal/storage"
)

// buildFixture builds and seeds the synthetic deployment. The store is
// opened here, in test code, per the ADR-027 read-surface guard.
func buildFixture(t *testing.T, opts fx.Options) *fx.Deployment {
	t.Helper()
	d, err := fx.Build(t.TempDir(), opts)
	if err != nil {
		t.Fatal(err)
	}
	st, err := storage.Open(d.Runtime.ProjectionPath(fx.Profile))
	if err != nil {
		t.Fatal(err)
	}
	err = d.Seed(st)
	if cerr := st.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestMain doubles as the fake model command (no shell script, so it works
// on every platform): with BEME_EVAL_FAKE_MODEL=1 the test binary reads the
// prompt from stdin, logs the call to BEME_EVAL_FAKE_MARKER, and echoes.
func TestMain(m *testing.M) {
	if os.Getenv("BEME_EVAL_FAKE_MODEL") == "1" {
		prompt, _ := io.ReadAll(os.Stdin)
		if marker := os.Getenv("BEME_EVAL_FAKE_MARKER"); marker != "" {
			if f, err := os.OpenFile(marker, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				fmt.Fprintln(f, len(prompt))
				f.Close()
			}
		}
		if os.Getenv("BEME_EVAL_FAKE_FAIL") == "1" {
			fmt.Fprintln(os.Stderr, "fake model failure")
			os.Exit(9)
		}
		fmt.Print("fake-model answer:\n")
		os.Stdout.Write(prompt)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type cliReport struct {
	ExitCode  int                `json:"exit_code"`
	Artifacts string             `json:"artifacts"`
	Summary   evalrunner.Summary `json:"summary"`
}

func deployment(t *testing.T) (cfg, corpus string) {
	t.Helper()
	d := buildFixture(t, fx.Options{})
	corpus = t.TempDir()
	if err := fx.WriteCorpus(corpus, d.AlphaPath, 1); err != nil {
		t.Fatal(err)
	}
	return d.ConfigDir, corpus
}

func fakeCommand(t *testing.T) (command, marker string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker = filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("BEME_EVAL_FAKE_MODEL", "1")
	t.Setenv("BEME_EVAL_FAKE_MARKER", marker)
	return `"` + exe + `" -test.run=^$`, marker
}

func calls(t *testing.T, marker string) int {
	t.Helper()
	f, err := os.Open(marker)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n := 0
	for s := bufio.NewScanner(f); s.Scan(); {
		n++
	}
	return n
}

func runJSON(t *testing.T, args ...string) (int, cliReport, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(append(args, "--json"), &out, &errb)
	var rep cliReport
	if code != 2 && strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatalf("invalid JSON report: %v\n%s", err, out.String())
		}
	}
	return code, rep, out.String() + errb.String()
}

// TestNoCorpusIsNotRun: without --corpus or BEME_PRIVATE_EVAL_DIR both
// commands report not_run and exit 3 (the private corpus is never assumed).
func TestNoCorpusIsNotRun(t *testing.T) {
	t.Setenv("BEME_PRIVATE_EVAL_DIR", "")
	cfg, _ := deployment(t)
	command, marker := fakeCommand(t)
	for _, args := range [][]string{
		{"retrieval", "--config", cfg},
		{"behavioral", "--config", cfg, "--provider", "command", "--command", command},
	} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 3 || !strings.Contains(out.String(), "not_run") {
			t.Fatalf("%v: exit %d, output %q", args, code, out.String()+errb.String())
		}
	}
	if calls(t, marker) != 0 {
		t.Fatal("no generation may run without a corpus")
	}
}

// TestRetrievalCommandMeasuresDeployment runs the deterministic retrieval
// measurement on the synthetic deployment and writes artifacts to --out.
func TestRetrievalCommandMeasuresDeployment(t *testing.T) {
	cfg, corpus := deployment(t)
	out := t.TempDir()
	code, rep, raw := runJSON(t, "retrieval", "--corpus", corpus, "--config", cfg, "--out", out)
	r := rep.Summary.Retrieval
	// both required refs are selected (recall 1); the fixture's irrelevant
	// records keep precision below the threshold, so the run fails honestly
	if r.Measured != 1 || r.Recall != 1 || r.Precision >= evalrunner.PrecisionThreshold || len(rep.Summary.CaseResults) != 0 {
		t.Fatalf("retrieval report: %+v\n%s", r, raw)
	}
	if code != 1 || rep.ExitCode != 1 {
		t.Fatalf("a below-threshold measurement must exit 1; got %d (%d)", code, rep.ExitCode)
	}
	if rep.Artifacts != filepath.Join(out, rep.Summary.RunID) {
		t.Fatalf("artifacts at %s, want under --out", rep.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(rep.Artifacts, "summary.json")); err != nil {
		t.Fatalf("summary.json missing: %v", err)
	}
}

// TestOutInsideRepositoryIsRefused: artifacts can never be written inside
// the repository checkout.
func TestOutInsideRepositoryIsRefused(t *testing.T) {
	cfg, corpus := deployment(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "eval-artifacts-must-not-exist")
	var out, errb bytes.Buffer
	if code := run([]string{"retrieval", "--corpus", corpus, "--config", cfg, "--out", target}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "inside the Be Me repository") {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		os.RemoveAll(target)
		t.Fatal("the refused directory must not be created")
	}
}

// TestBehavioralDryRunDoesNotExecute: --dry-run renders every prompt and
// manifest, reports the generation count, and never runs the command.
func TestBehavioralDryRunDoesNotExecute(t *testing.T) {
	cfg, corpus := deployment(t)
	command, marker := fakeCommand(t)
	out := t.TempDir()
	code, rep, raw := runJSON(t, "behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command,
		"--dry-run", "--experimental-learned", "--repeats", "2", "--out", out)
	want := 2 * len(evalrunner.AllArms)
	if code != 3 || rep.Summary.PlannedGenerations != want || rep.Summary.Generations != 0 {
		t.Fatalf("dry run: exit %d planned %d generations %d (want 3/%d/0)\n%s", code, rep.Summary.PlannedGenerations, rep.Summary.Generations, want, raw)
	}
	if n := calls(t, marker); n != 0 {
		t.Fatalf("dry run executed the command %d times", n)
	}
	manifests, _ := os.ReadDir(filepath.Join(rep.Artifacts, "manifests"))
	rawFiles, _ := os.ReadDir(filepath.Join(rep.Artifacts, "raw"))
	if len(manifests) != want || len(rawFiles) != want {
		t.Fatalf("dry run artifacts: %d manifests, %d raw (want %d)", len(manifests), len(rawFiles), want)
	}

	var text, errb bytes.Buffer
	run([]string{"behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--dry-run", "--arms", "B0,B4", "--out", t.TempDir()}, &text, &errb)
	if !strings.Contains(text.String(), "dry run: 2 generation(s) would run; none executed") {
		t.Fatalf("text report must state the planned generations:\n%s%s", text.String(), errb.String())
	}
}

// TestBehavioralRunsCommandProvider: each generation pipes the rendered
// prompt to the command and grades its stdout; artifacts carry the text and
// the declared model settings.
func TestBehavioralRunsCommandProvider(t *testing.T) {
	cfg, corpus := deployment(t)
	command, marker := fakeCommand(t)
	code, rep, raw := runJSON(t, "behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command,
		"--arms", "B0,B4,no-scope", "--repeats", "1", "--model-id", "fake-model", "--temperature", "0", "--out", t.TempDir())
	if code != 0 || rep.Summary.Generations != 3 {
		t.Fatalf("exit %d, generations %d\n%s", code, rep.Summary.Generations, raw)
	}
	if n := calls(t, marker); n != 3 {
		t.Fatalf("command ran %d times, want 3", n)
	}
	for _, cr := range rep.Summary.CaseResults {
		if cr.Outcome != evalrunner.OutcomePassed || !strings.HasPrefix(cr.PerRepeat[0].Text, "fake-model answer:\n") || !strings.Contains(cr.PerRepeat[0].Text, "=== TASK ===") {
			t.Fatalf("%s: %+v", cr.Arm, cr)
		}
	}
	manifests, _ := os.ReadDir(filepath.Join(rep.Artifacts, "manifests"))
	if len(manifests) != 3 {
		t.Fatalf("manifests: %d", len(manifests))
	}
	mb, _ := os.ReadFile(filepath.Join(rep.Artifacts, "manifests", manifests[0].Name()))
	var m map[string]any
	json.Unmarshal(mb, &m)
	if m["model_provider"] != "command" || m["model_id"] != "fake-model" || m["model_version"] != "unreported" || m["top_p"] != -1.0 {
		t.Fatalf("manifest must record the declared settings: %v", m)
	}
	blinded, _ := os.ReadDir(filepath.Join(rep.Artifacts, "blinded"))
	for _, f := range blinded {
		b, _ := os.ReadFile(filepath.Join(rep.Artifacts, "blinded", f.Name()))
		if !strings.Contains(string(b), "fake-model answer") || strings.Contains(string(b), `"B4"`) || strings.Contains(string(b), `"no-scope"`) {
			t.Fatalf("blinded artifact %s: %s", f.Name(), b)
		}
	}

	t.Setenv("BEME_EVAL_FAKE_FAIL", "1")
	code, rep, raw = runJSON(t, "behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--arms", "B0", "--out", t.TempDir())
	if code != 1 || rep.Summary.CaseResults[0].Outcome != evalrunner.OutcomeFailed || !strings.Contains(rep.Summary.CaseResults[0].Reason, "fake model failure") {
		t.Fatalf("a failing command must fail the unit and exit 1: %d\n%s", code, raw)
	}
}

func TestUsageErrors(t *testing.T) {
	cfg, corpus := deployment(t)
	command, _ := fakeCommand(t)
	cases := [][]string{
		{},
		{"bogus"},
		{"retrieval", "--bogus"},
		{"retrieval", "--corpus", corpus},
		{"retrieval", "--corpus", corpus, "--config", filepath.Join(t.TempDir(), "missing")},
		{"behavioral", "--corpus", corpus, "--config", cfg, "--provider", "mock", "--command", command},
		{"behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command"},
		{"behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--arms", "B9"},
		{"behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--repeats", "9"},
		{"behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", `"unterminated`},
	}
	for _, args := range cases {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 2 {
			t.Errorf("%q: exit %d, want 2 (%s)", args, code, errb.String())
		}
	}
}
