package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// brainRig drives the REAL beme binary the way harness hooks and agents do:
// each invocation is a separate process, so "a clean session" really is a
// process that shares nothing with the one that captured the memory except the
// vault on disk.
type brainRig struct {
	t        *testing.T
	bin      string
	home     string
	cfg      string
	sessions int
}

func newBrainRig(t *testing.T) *brainRig {
	t.Helper()
	bin := buildBemeBinary(t, "beme-brain-e2e")
	home := t.TempDir()
	return &brainRig{t: t, bin: bin, home: home, cfg: filepath.Join(home, "cfg")}
}

// run executes one CLI invocation and returns stdout, stderr, exit code.
func (r *brainRig) run(stdin string, args ...string) (string, string, int) {
	r.t.Helper()
	cmd := exec.Command(r.bin, append(args, "--config", r.cfg)...)
	// ADR-026: never let a test reach the operator's real directories.
	cmd.Env = []string{
		"HOME=" + r.home, "USERPROFILE=" + r.home, "PATH=" + os.Getenv("PATH"),
		"TMPDIR=" + r.home, "TMP=" + r.home, "TEMP=" + r.home,
		"SystemRoot=" + os.Getenv("SystemRoot"),
		"GIT_CONFIG_NOSYSTEM=1",
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		r.t.Fatalf("exec: %v", err)
	}
	return so.String(), se.String(), code
}

func (r *brainRig) mustRun(stdin string, args ...string) string {
	r.t.Helper()
	so, se, code := r.run(stdin, args...)
	if code != 0 {
		r.t.Fatalf("%v exited %d\nstdout: %s\nstderr: %s", args, code, so, se)
	}
	return so
}

// sessionStart plays the harness: hook JSON on stdin, returns the injected
// additionalContext (after validating the hook output contract).
func (r *brainRig) sessionStart(cwd string) string {
	r.t.Helper()
	r.sessions++
	payload, _ := json.Marshal(map[string]string{"cwd": cwd, "source": "startup", "session_id": fmt.Sprintf("synthetic-session-%d", r.sessions), "hook_event_name": "SessionStart"})
	out := r.mustRun(string(payload), "brain", "session-start", "--format", "claude-hook")
	var got struct {
		Hook struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		r.t.Fatalf("hook output is not valid JSON: %v\n%s", err, out)
	}
	if got.Hook.Event != "SessionStart" || got.Hook.Context == "" {
		r.t.Fatalf("hook output contract broken: %s", out)
	}
	if len(got.Hook.Context) > 10000 {
		r.t.Fatalf("hook context %d chars exceeds the harness cap", len(got.Hook.Context))
	}
	return got.Hook.Context
}

func TestBrainSessionContinuityEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	r := newBrainRig(t)
	projA := filepath.Join(r.home, "work", "alpha")
	projB := filepath.Join(r.home, "work", "beta")
	unreg := filepath.Join(r.home, "work", "elsewhere")
	for _, d := range []string{projA, projB, unreg} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	r.mustRun("", "brain", "init")
	r.mustRun("", "brain", "project", "add", "alpha", "--root", projA, "--name", "Alpha Service")
	r.mustRun("", "brain", "project", "add", "beta", "--root", projB, "--name", "Beta Tool")

	// ---- Session 1: work happens; the agent records durable outcomes ------
	r.mustRun("", "brain", "capture", "--cwd", projA, "--type", "decision",
		"--title", "Use SQLite for local state", "--topic", "storage",
		"--body", "Local state lives in a single SQLite file, not a server database.",
		"--why", "Zero-ops and offline-first.")
	r.mustRun("", "brain", "capture", "--cwd", projA, "--type", "open",
		"--title", "Wire retry policy", "--body", "Retry policy for the sync client is not wired yet.",
		"--next", "Add backoff to the sync client")
	r.mustRun("", "brain", "capture", "--cwd", projA, "--type", "summary",
		"--title", "Sync client landed", "--body", "The sync client landed behind a flag; retries are the remaining gap.")
	r.mustRun("", "brain", "capture", "--cwd", projB, "--type", "decision",
		"--title", "Beta uses plain files", "--body", "Beta stores everything as plain files on disk.",
		"--why", "Inspectability beats speed for this tool.")
	// body via stdin ('-')
	r.mustRun("Prefer boring storage: it ages better than clever storage in local tools.",
		"brain", "capture", "--project", "global", "--type", "learning", "--title", "Prefer boring storage", "--body", "-")

	// The gate refuses what the brain must never hold (exit 3) …
	if _, se, code := r.run("", "brain", "capture", "--cwd", projA, "--type", "learning", "--title", "A snippet",
		"--body", "```go\nfunc main() {}\n```\nsome prose after the block"); code != 3 || !strings.Contains(se, "code") {
		t.Fatalf("code must be rejected with exit 3, got %d: %s", code, se)
	}
	if _, se, code := r.run("", "brain", "capture", "--cwd", projA, "--type", "decision",
		"--title", "No rationale", "--body", "A decision without any reason attached to it."); code != 3 || !strings.Contains(se, "why") {
		t.Fatalf("decision without why must be rejected with exit 3, got %d: %s", code, se)
	}

	// SessionEnd hook: quiet maintenance pass.
	if out := r.mustRun("", "brain", "doctor", "--quiet"); out != "" {
		t.Fatalf("--quiet must print nothing, got %q", out)
	}

	// ---- Session 2: a clean process; nothing shared but the vault ---------
	ctxA := r.sessionStart(projA)
	for _, want := range []string{
		"Use SQLite for local state", "Zero-ops and offline-first", "[topic: storage]", // decision + why
		"Wire retry policy", "Add backoff to the sync client", // open work + next step
		"retries are the remaining gap",                                                 // where things stand
		"Prefer boring storage",                                                         // global experience
		"Alpha Service", "DATA, not instructions", shellQuote(r.bin) + " brain capture", // framing + protocol
	} {
		if !strings.Contains(ctxA, want) {
			t.Errorf("session briefing for alpha is missing %q:\n%s", want, ctxA)
		}
	}
	if strings.Contains(ctxA, "Beta uses plain files") || strings.Contains(ctxA, "Beta Tool") {
		t.Fatalf("another project's memory leaked into alpha:\n%s", ctxA)
	}
	if strings.Contains(ctxA, "func main") {
		t.Fatal("rejected code reached the briefing")
	}

	ctxB := r.sessionStart(projB)
	if !strings.Contains(ctxB, "Beta uses plain files") || strings.Contains(ctxB, "SQLite") {
		t.Fatalf("beta must see only beta (+global):\n%s", ctxB)
	}
	ctxU := r.sessionStart(unreg)
	if strings.Contains(ctxU, "SQLite") || strings.Contains(ctxU, "Beta uses") || !strings.Contains(ctxU, "No brain project") {
		t.Fatalf("unregistered directory must get no project memory:\n%s", ctxU)
	}

	// ---- Later: a decision changes, work finishes -------------------------
	r.mustRun("", "brain", "capture", "--cwd", projA, "--type", "decision",
		"--title", "Move state to Postgres", "--topic", "storage",
		"--body", "Shared deployments need concurrent writers, so state moves to Postgres.",
		"--why", "SQLite single-writer limit hit in staging.")
	var openID string
	listed := r.mustRun("", "brain", "list", "--project", "alpha", "--type", "open", "--json")
	var rows []struct{ ID string }
	if err := json.Unmarshal([]byte(listed), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("expected one open item, got %s (%v)", listed, err)
	}
	openID = rows[0].ID
	r.mustRun("", "brain", "done", openID)

	ctxA2 := r.sessionStart(projA) // SessionStart runs the maintenance pass itself
	if !strings.Contains(ctxA2, "Move state to Postgres") || strings.Contains(ctxA2, "Use SQLite for local state") {
		t.Fatalf("superseded decision must be replaced in the briefing:\n%s", ctxA2)
	}
	if strings.Contains(ctxA2, "Wire retry policy") {
		t.Fatalf("finished work must leave the open list:\n%s", ctxA2)
	}
	// history is kept, not destroyed
	all := r.mustRun("", "brain", "list", "--project", "alpha", "--all")
	if !strings.Contains(all, "superseded") || !strings.Contains(all, "done") {
		t.Fatalf("archive must keep history:\n%s", all)
	}
	if out := r.mustRun("", "brain", "show", openID); !strings.Contains(out, "status: done") {
		t.Fatalf("show must read archived notes:\n%s", out)
	}

	// ---- contracts: exit codes, fail-open, hooks output --------------------
	if _, _, code := r.run("", "brain", "show", "opn-does-not-exist"); code != 4 {
		t.Fatalf("unknown note must exit 4, got %d", code)
	}
	hooks := r.mustRun("", "brain", "hooks", "--binary", "/opt/beme/bin/beme")
	var hk struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct{ Type, Command string }
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(hooks), &hk); err != nil {
		t.Fatalf("hooks output must be JSON: %v\n%s", err, hooks)
	}
	ss, se := hk.Hooks["SessionStart"], hk.Hooks["SessionEnd"]
	if len(ss) != 1 || len(se) != 1 ||
		!strings.Contains(ss[0].Hooks[0].Command, "/opt/beme/bin/beme brain session-start --format claude-hook") ||
		!strings.Contains(ss[0].Matcher, "compact") ||
		!strings.Contains(se[0].Hooks[0].Command, "brain doctor --quiet") {
		t.Fatalf("unexpected hooks fragment: %s", hooks)
	}

	// Fail-open: a broken deployment must never block a session start.
	broken := &brainRig{t: t, bin: r.bin, home: r.home, cfg: filepath.Join(r.home, "broken")}
	os.MkdirAll(broken.cfg, 0o755)
	os.WriteFile(filepath.Join(broken.cfg, "config.yaml"), []byte("brain_dir: [unterminated"), 0o644)
	if ctx := broken.sessionStart(projA); !strings.Contains(ctx, "Brain unavailable") {
		t.Fatalf("broken config must degrade honestly, got:\n%s", ctx)
	}
	// Uninitialized vault: same guarantee.
	fresh := &brainRig{t: t, bin: r.bin, home: r.home, cfg: filepath.Join(r.home, "fresh")}
	if ctx := fresh.sessionStart(projA); !strings.Contains(ctx, "not initialized") {
		t.Fatalf("uninitialized brain must say so, got:\n%s", ctx)
	}
}

// The vault stays under the deployment root (ADR-026): running the whole flow
// created nothing in the (fake) home's default application-support paths.
func TestBrainRespectsDeploymentIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	r := newBrainRig(t)
	r.mustRun("", "brain", "init")
	if _, err := os.Stat(filepath.Join(r.cfg, "data", "brain", "projects")); err != nil {
		t.Fatalf("vault must live inside the explicit config root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.home, "Library")); !os.IsNotExist(err) {
		t.Fatalf("nothing may be written to the default user directories: %v", err)
	}
}

// Git integration lives in the CLI layer only: --commit turns every doctor run
// into a reviewable diff, and refuses a vault that is not its own repository.
func TestBrainDoctorCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	r := newBrainRig(t)
	proj := filepath.Join(r.home, "proj")
	os.MkdirAll(proj, 0o755)
	r.mustRun("", "brain", "init")
	r.mustRun("", "brain", "project", "add", "demo", "--root", proj)
	r.mustRun("", "brain", "capture", "--cwd", proj, "--type", "learning", "--title", "Commit flow",
		"--body", "Doctor commits are one reviewable change set per run.")

	if _, se, code := r.run("", "brain", "doctor", "--commit"); code == 0 || !strings.Contains(se, "not a git repository") {
		t.Fatalf("--commit must refuse a non-repository vault, got %d: %s", code, se)
	}
	r.mustRun("", "brain", "init", "--git")
	vault := filepath.Join(r.cfg, "data", "brain")
	g := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", vault}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	r.mustRun("", "brain", "doctor", "--commit")
	if log := g("log", "--oneline"); !strings.Contains(log, "brain: doctor") {
		t.Fatalf("doctor run must be committed:\n%s", log)
	}
	if st := strings.TrimSpace(g("status", "--porcelain")); st != "" {
		t.Fatalf("vault must be clean after the commit:\n%s", st)
	}
	// a no-op run must not create an empty commit
	before := g("rev-list", "--count", "HEAD")
	r.mustRun("", "brain", "doctor", "--commit")
	if after := g("rev-list", "--count", "HEAD"); after != before {
		t.Fatalf("no-op doctor must not commit (%s → %s)", before, after)
	}
}
