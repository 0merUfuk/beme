package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
)

func buildBemeBinary(t *testing.T, name string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), name)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = mustRepoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s", out)
	}
	return bin
}

func runBeme(t *testing.T, bin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return stdout.String(), stderr.String(), ee.ExitCode()
	}
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return stdout.String(), stderr.String(), 0
}

func diagnosticsDeployment(t *testing.T) (string, *app.Runtime) {
	t.Helper()
	home := t.TempDir()
	cfg := filepath.Join(home, "cfg")
	entries := filepath.Join(home, "src", "entries")
	for _, d := range []string{filepath.Join(cfg, "sources"), entries} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(entries, "DIAG-001.md"), []byte("---\nid: DIAG-001\ntitle: \"Diag\"\ntype: preference\nstatus: active\n---\n\nDiagnostics fixture statement.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "sources", "s.yaml"), []byte("schema_version: \"1\"\nsource_id: diag-src\ntype: directory\nroot: "+filepath.ToSlash(filepath.Join(home, "src"))+"\npurpose: [reusable_knowledge]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: default\nsensitivity: personal_private\nprofiles_allowed: [personal]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rt, err := app.Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, rt
}

// TestHelpAndVersionExitZero: asking for help or the version is not a usage
// error. It is a new user's first command, and the documented exit-code
// contract reserves 2 for actual misuse.
func TestHelpAndVersionExitZero(t *testing.T) {
	bin := buildBemeBinary(t, "beme-help")
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}, {"version"}, {"--version"}, {"-v"}} {
		stdout, stderr, code := runBeme(t, bin, args...)
		if code != 0 {
			t.Errorf("%v exited %d, want 0 (stderr: %s)", args, code, stderr)
		}
		if !strings.Contains(stdout, "beme "+version) {
			t.Errorf("%v must print the version on stdout; got %q", args, stdout)
		}
	}
	// a genuine misuse still exits 2
	if _, _, code := runBeme(t, bin, "no-such-command"); code != 2 {
		t.Errorf("an unknown command must exit 2; got %d", code)
	}
}

// TestPreviewPrintsTheTraceExplainAccepts: the human preview must print the
// trace ID `beme explain` needs, and explaining with exactly that ID must work
// (an onboarding pass found no documented way to obtain it).
func TestPreviewPrintsTheTraceExplainAccepts(t *testing.T) {
	bin := buildBemeBinary(t, "beme-trace")
	cfg, _ := diagnosticsDeployment(t)
	if _, stderr, code := runBeme(t, bin, "build", "--profile", "personal", "--config", cfg); code != 0 {
		t.Fatalf("build: %d %s", code, stderr)
	}
	out, stderr, code := runBeme(t, bin, "preview", "--task", "diagnostics fixture", "--config", cfg)
	if code != 0 {
		t.Fatalf("preview: %d %s", code, stderr)
	}
	var traceID string
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "trace: "); ok {
			traceID, _, _ = strings.Cut(rest, " ")
		}
	}
	if !strings.HasPrefix(traceID, "trace_") {
		t.Fatalf("human preview must print the trace ID; output:\n%s", out)
	}
	if out, stderr, code := runBeme(t, bin, "explain", "--projection", "personal", "--trace", traceID, "--config", cfg); code != 0 || out == "" {
		t.Fatalf("explain with the printed trace ID must succeed: %d %s %s", code, out, stderr)
	}
}

// TestDoctorKeepsMostSevereStatus: an interrupted purge (degraded) must not
// mask an unusable ledger (policy_blocked). Doctor is a diagnostic: it exits 0
// and reports the state in its output.
func TestDoctorKeepsMostSevereStatus(t *testing.T) {
	bin := buildBemeBinary(t, "beme-doctor")
	cfg, rt := diagnosticsDeployment(t)
	if _, stderr, code := runBeme(t, bin, "build", "--profile", "personal", "--config", cfg); code != 0 {
		t.Fatalf("build: %d %s", code, stderr)
	}
	if _, stderr, code := runBeme(t, bin, "purge", "--config", cfg, "--confirm", "rec_diag-001", "rec_diag-001"); code != 0 {
		t.Fatalf("purge: %d %s", code, stderr)
	}
	pending := filepath.Join(filepath.Dir(rt.LedgerPath()), "pending", "0123456789abcdef01234567.json")
	if err := os.MkdirAll(filepath.Dir(pending), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pending, []byte(`{"schema_version":"1","records":[],"traces":[],"observations":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, code := runBeme(t, bin, "doctor", "--json", "--config", cfg)
	var rep struct {
		Status   string   `json:"status"`
		Findings []string `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil || code != 0 {
		t.Fatalf("doctor: code=%d err=%v out=%s", code, err, out)
	}
	if rep.Status != "degraded" || !strings.Contains(strings.Join(rep.Findings, " "), "interrupted physical purge") {
		t.Fatalf("positive control: a pending purge alone is degraded; got %+v", rep)
	}
	if err := os.Remove(rt.PurgeKeyPath()); err != nil {
		t.Fatal(err)
	}
	out, _, code = runBeme(t, bin, "doctor", "--json", "--config", cfg)
	if err := json.Unmarshal([]byte(out), &rep); err != nil || code != 0 {
		t.Fatalf("doctor: code=%d err=%v out=%s", code, err, out)
	}
	if rep.Status != "policy_blocked" {
		t.Fatalf("an unusable ledger with a pending purge must stay policy_blocked; got %s", rep.Status)
	}
	_ = contracts.ProfilePersonal
}

// TestExplainErrorClasses: only an unusable ledger is reported as policy
// blocked (exit 3); an ordinary failure is an error (exit 1); an unknown trace
// is not found (exit 4).
func TestExplainErrorClasses(t *testing.T) {
	bin := buildBemeBinary(t, "beme-explain")
	cfg, rt := diagnosticsDeployment(t)
	if _, stderr, code := runBeme(t, bin, "build", "--profile", "personal", "--config", cfg); code != 0 {
		t.Fatalf("build: %d %s", code, stderr)
	}
	_, stderr, code := runBeme(t, bin, "explain", "--trace", "trace_000000", "--config", cfg)
	if code != 4 || !strings.Contains(stderr, "trace not available") {
		t.Fatalf("unknown trace: want exit 4, got %d %q", code, stderr)
	}
	_, stderr, code = runBeme(t, bin, "explain", "--trace", "trace_000000", "--projection", "bogus", "--config", cfg)
	if code != 1 || strings.Contains(stderr, "policy blocked") || !strings.HasPrefix(stderr, "error:") {
		t.Fatalf("an ordinary failure must be exit 1 and not labeled policy blocked; got %d %q", code, stderr)
	}
	if err := os.MkdirAll(filepath.Dir(rt.LedgerPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rt.LedgerPath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runBeme(t, bin, "explain", "--trace", "trace_000000", "--config", cfg)
	if code != 3 || !strings.HasPrefix(stderr, "policy blocked:") {
		t.Fatalf("an unusable ledger must be exit 3 and labeled policy blocked; got %d %q", code, stderr)
	}
}
