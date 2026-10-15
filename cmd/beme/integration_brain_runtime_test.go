package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/resolver"
)

func TestBrainUsageDescribesReadOnlyBriefing(t *testing.T) {
	if testing.Short() {
		t.Skip("real-binary help")
	}
	bin := buildBemeBinary(t, "beme-brain-help")
	_, usage, code := runBeme(t, bin, "brain")
	if code != 2 {
		t.Fatalf("missing subcommand must report usage: %d", code)
	}
	for _, line := range strings.Split(usage, "\n") {
		if strings.Contains(line, "beme brain briefing") {
			if !strings.Contains(line, "--doctor") || !strings.Contains(line, "read-only") || strings.Contains(line, "= session-start") {
				t.Fatalf("briefing help must describe the true read and opt-in maintenance: %s", line)
			}
			return
		}
	}
	t.Fatal("missing briefing usage")
}

// One binary must support the adopted brain without replacing the newer
// runtime. Every invocation is a fresh process using synthetic data only.
func TestIntegratedBrainAndRuntime(t *testing.T) {
	if testing.Short() {
		t.Skip("real-binary integration")
	}
	bin := buildBemeBinary(t, "beme-integrated")
	cfg, rt := diagnosticsDeployment(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, key := range []string{"BEME_CONFIG_HOME", "BEME_DATA_HOME", "BEME_CACHE_HOME", "BEME_BRAIN_HOME"} {
		t.Setenv(key, filepath.Join(home, key))
	}
	must := func(args ...string) string {
		t.Helper()
		out, stderr, code := runBeme(t, bin, append(args, "--config", cfg)...)
		if code != 0 {
			t.Fatalf("%v: exit %d\nstdout=%s\nstderr=%s", args, code, out, stderr)
		}
		return out
	}

	must("brain", "init")
	must("brain", "project", "add", "demo", "--root", home)
	const brainTitle = "Synthetic continuity sentinel"
	must("brain", "capture", "--project", "demo", "--type", "decision", "--title", brainTitle,
		"--body", "A separate work memory vault retains decisions across sessions.",
		"--why", "Continuity without changing canonical runtime evidence.")
	must("brain", "doctor", "--quiet")
	if out := must("brain", "briefing", "--cwd", home); !strings.Contains(out, brainTitle) {
		t.Fatalf("the same binary must recall its capture in a new process: %s", out)
	}
	if _, err := os.Stat(filepath.Join(cfg, "data", "brain", "brain.yaml")); err != nil {
		t.Fatalf("brain must default inside the explicit config root: %v", err)
	}
	if _, err := os.Stat(os.Getenv("BEME_BRAIN_HOME")); !os.IsNotExist(err) {
		t.Fatalf("explicit config must not use BEME_BRAIN_HOME: %v", err)
	}

	must("build", "--profile", "all", "--json")
	preview := func(profile, task string) resolver.Pack {
		t.Helper()
		out := must("preview", "--projection", profile, "--task", task, "--json")
		if strings.Contains(out, brainTitle) {
			t.Fatal("brain notes must never be injected into V1 packs")
		}
		var pack resolver.Pack
		if err := json.Unmarshal([]byte(out), &pack); err != nil {
			t.Fatal(err)
		}
		return pack
	}
	positive := preview("personal", "diagnostics fixture")
	if len(positive.Guidance) != 1 || positive.Guidance[0].RecordID != "rec_diag-001" {
		t.Fatalf("positive runtime control must retrieve its registered source: %+v", positive)
	}
	negative := preview("personal", "Which database should the billing service use?")
	if len(negative.Guidance)+len(negative.Precedents)+len(negative.LearnedExperimental) != 0 {
		t.Fatalf("PR2 must drop irrelevant advisory records: %+v", negative)
	}
	foundUnknown := false
	for _, u := range negative.Unknowns {
		foundUnknown = foundUnknown || u.Question == resolver.NoEvidenceQuestion
	}
	if !foundUnknown {
		t.Fatalf("PR2 negative relevance must state the lack of evidence: %+v", negative.Unknowns)
	}
	if out := must("explain", "--projection", "personal", "--trace", negative.TraceRef, "--json"); !strings.Contains(out, "not relevant to task") {
		t.Fatalf("PR2's actual relevance trace must remain available: %s", out)
	}
	safe := preview("work-safe", "diagnostics fixture continuity decisions")
	if len(safe.Guidance)+len(safe.Precedents)+len(safe.Constraints)+len(safe.LearnedExperimental) != 0 {
		t.Fatalf("private runtime and brain content must be absent from work-safe: %+v", safe)
	}

	// The old V2 fork forgot only inside the rebuildable projection. The
	// newer runtime must still persist enforcement in the durable ledger.
	out, stderr, code := runBeme(t, bin, "forget", "--config", cfg, "rec_diag-001")
	if code != 0 {
		t.Fatalf("forget: %d %s %s", code, out, stderr)
	}
	if _, err := os.Stat(rt.LedgerPath()); err != nil {
		t.Fatalf("forget must preserve the newer durable ledger: %v", err)
	}
	must("build", "--profile", "personal")
	if pack := preview("personal", "diagnostics fixture"); len(pack.Guidance) != 0 {
		t.Fatalf("rebuild must not resurrect forgotten records: %+v", pack)
	}

	// Brain config resolution must not inherit V1 registration failures.
	if err := os.WriteFile(filepath.Join(cfg, "sources", "broken.yaml"), []byte("root: [unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := must("brain", "briefing", "--cwd", home); !strings.Contains(out, brainTitle) {
		t.Fatalf("malformed V1 source must not block brain read: %s", out)
	}
	if _, _, code := runBeme(t, bin, "build", "--config", cfg); code == 0 {
		t.Fatal("malformed V1 source must still fail runtime loading")
	}
}
