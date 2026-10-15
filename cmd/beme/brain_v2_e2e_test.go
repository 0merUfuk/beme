package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Operator-facing lifecycle of the V2 additions, through the real binary:
// status, snapshots + restore, freeze, modes, strict lint, session-silence
// nudge, install-hooks.
func TestBrainOperatorLifecycleEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	r := newBrainRig(t)
	proj := filepath.Join(r.home, "work", "alpha")
	os.MkdirAll(proj, 0o755)
	r.mustRun("", "brain", "init")
	r.mustRun("", "brain", "project", "add", "alpha", "--root", proj, "--name", "Alpha Service")

	// ---- nudge: two real session starts with no capture ------------------
	if ctx := r.sessionStart(proj); strings.Contains(ctx, "recorded nothing") {
		t.Fatal("first session must not nudge")
	}
	r.sessionStart(proj)
	if ctx := r.sessionStart(proj); !strings.Contains(ctx, "the last 2 start/resume/fork invocations in this project recorded nothing") {
		t.Fatalf("third session must be nudged:\n%s", ctx)
	}
	r.mustRun("", "brain", "capture", "--cwd", proj, "--type", "decision", "--title", "Use SQLite for local state",
		"--topic", "storage", "--body", "Local state lives in a single SQLite file, not a server database.", "--why", "Zero-ops and offline-first.")
	if ctx := r.sessionStart(proj); strings.Contains(ctx, "recorded nothing") {
		t.Fatalf("a capture must clear the nudge:\n%s", ctx)
	}

	// ---- status ------------------------------------------------------------
	var st struct {
		Initialized bool           `json:"initialized"`
		Frozen      bool           `json:"frozen"`
		Live        map[string]int `json:"live_by_type"`
		Snapshots   int            `json:"snapshots"`
		LastLog     string         `json:"last_log_entry"`
	}
	if err := json.Unmarshal([]byte(r.mustRun("", "brain", "status", "--json")), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Initialized || st.Live["decision"] != 1 || st.Snapshots < 1 || !strings.Contains(st.LastLog, "promoted") {
		t.Fatalf("unexpected status: %+v", st)
	}
	if out := r.mustRun("", "brain", "status"); !strings.Contains(out, "decision 1") {
		t.Fatalf("human status:\n%s", out)
	}

	// ---- snapshot → damage → restore --------------------------------------
	snapOut := r.mustRun("", "brain", "backup")
	snapPath := strings.TrimSpace(strings.TrimPrefix(snapOut, "snapshot written:"))
	if !strings.HasSuffix(snapPath, ".tar.gz") {
		t.Fatalf("snapshot path: %q", snapOut)
	}
	listed := r.mustRun("", "brain", "list", "--project", "alpha", "--json")
	var rows []struct{ ID string }
	json.Unmarshal([]byte(listed), &rows)
	r.mustRun("", "brain", "archive", rows[0].ID, "--reason", "accident")
	if strings.Contains(r.sessionStart(proj), "Use SQLite for local state") {
		t.Fatal("setup: archived decision must be gone from the briefing")
	}
	if _, se, code := r.run("", "brain", "restore", snapPath); code == 0 || !strings.Contains(se, "--force") {
		t.Fatalf("restore over a live vault needs --force, got %d: %s", code, se)
	}
	r.mustRun("", "brain", "restore", snapPath, "--force")
	if !strings.Contains(r.sessionStart(proj), "Use SQLite for local state") {
		t.Fatal("restore must bring the decision back")
	}
	if out := r.mustRun("", "brain", "backup", "--list"); !strings.Contains(out, "pre-restore") {
		t.Fatalf("restore must leave a pre-restore snapshot:\n%s", out)
	}

	// ---- freeze ------------------------------------------------------------
	r.mustRun("", "brain", "freeze", "on")
	if _, se, code := r.run("", "brain", "capture", "--cwd", proj, "--type", "learning", "--title", "While frozen",
		"--body", "Nothing may be written while the brain is frozen."); code == 0 || !strings.Contains(se, "frozen") {
		t.Fatalf("capture must be refused while frozen, got %d: %s", code, se)
	}
	if ctx := r.sessionStart(proj); !strings.Contains(ctx, "FROZEN") || !strings.Contains(ctx, "Use SQLite for local state") {
		t.Fatalf("frozen brain must still brief (read-only) and say so:\n%s", ctx)
	}
	r.mustRun("", "brain", "freeze", "off")
	r.mustRun("", "brain", "capture", "--cwd", proj, "--type", "learning", "--title", "After unfreeze",
		"--body", "Writes resume once the operator unfreezes the brain.")

	// ---- modes: config default and flag override ---------------------------
	os.WriteFile(filepath.Join(r.cfg, "config.yaml"), []byte("brain_mode: manual\n"), 0o600)
	if ctx := r.sessionStart(proj); !strings.Contains(ctx, "Manual mode") || strings.Contains(ctx, "Local state lives") {
		t.Fatalf("config brain_mode: manual must yield the pull-model briefing:\n%s", ctx)
	}
	if out := r.mustRun("", "brain", "briefing", "--cwd", proj, "--mode", "normal"); !strings.Contains(out, "Local state lives") {
		t.Fatalf("`briefing --mode normal` must pull the full briefing:\n%s", out)
	}
	if _, se, _ := r.run("", "brain", "briefing", "--cwd", proj, "--mode", "turbo"); !strings.Contains(se, "unknown brain mode") {
		t.Fatalf("bad mode must be reported: %s", se)
	}
	os.Remove(filepath.Join(r.cfg, "config.yaml"))

	// ---- strict lint --------------------------------------------------------
	r.mustRun("", "brain", "doctor", "--strict") // clean vault passes
	os.WriteFile(filepath.Join(r.cfg, "data", "brain", "helper.py"), []byte("print(1)"), 0o600)
	if _, _, code := r.run("", "brain", "doctor", "--strict"); code != 1 {
		t.Fatalf("--strict must exit 1 on a flag, got %d", code)
	}
	os.Remove(filepath.Join(r.cfg, "data", "brain", "helper.py"))

	// ---- hooks fragment has all three events; install-hooks round trip ------
	var hk struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	json.Unmarshal([]byte(r.mustRun("", "brain", "hooks", "--binary", "/opt/beme")), &hk)
	for _, ev := range []string{"SessionStart", "PreCompact", "SessionEnd"} {
		if _, ok := hk.Hooks[ev]; !ok {
			t.Errorf("hooks fragment missing %s", ev)
		}
	}
	settings := filepath.Join(r.home, "settings.json")
	os.WriteFile(settings, []byte(`{"model":"opus"}`), 0o600)
	if out := r.mustRun("", "brain", "install-hooks", "--settings", settings, "--binary", "/opt/beme"); !strings.Contains(out, "installed") {
		t.Fatalf("install-hooks: %s", out)
	}
	if out := r.mustRun("", "brain", "install-hooks", "--settings", settings, "--binary", "/opt/beme"); !strings.Contains(out, "already up to date") {
		t.Fatalf("install-hooks must be idempotent: %s", out)
	}
	if _, _, code := r.run("", "brain", "install-hooks"); code != 2 {
		t.Fatalf("install-hooks without a target must be a usage error, got %d", code)
	}
	r.mustRun("", "brain", "install-hooks", "--settings", settings, "--remove")
	if b, _ := os.ReadFile(settings); strings.Contains(string(b), "brain") || !strings.Contains(string(b), "opus") {
		t.Fatalf("--remove must leave only the user's settings: %s", b)
	}
}
