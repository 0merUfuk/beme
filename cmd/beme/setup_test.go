package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/0merUfuk/beme/internal/setup"
	"testing"
)

// runSetup runs the real binary with HOME pointed at a temporary directory,
// so no test can touch a real harness file.
func runSetup(t *testing.T, bin, home, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home,
		"BEME_CONFIG_HOME=", "BEME_DATA_HOME=", "BEME_CACHE_HOME=")
	cmd.Stdin = strings.NewReader(stdin)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return so.String(), se.String(), ee.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return so.String(), se.String(), 0
}

func setupHome(t *testing.T, claudeMD string) (home, cfg, target string) {
	t.Helper()
	home = t.TempDir()
	cfg = filepath.Join(home, "cfg")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	target = filepath.Join(home, ".claude", "CLAUDE.md")
	if claudeMD != "" {
		if err := os.WriteFile(target, []byte(claudeMD), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return
}

func countFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			n++
		}
		return nil
	})
	return n
}

const userClaude = "# My notes\n\nKeep this exactly.\n"

func TestSetupDryRunWritesNothing(t *testing.T) {
	bin := buildBemeBinary(t, "beme")
	home, cfg, target := setupHome(t, userClaude)
	before := countFiles(t, home)
	out, _, code := runSetup(t, bin, home, "", "setup", "--defaults", "--harness", "claude-code", "--config", cfg)
	if code != 0 || !strings.Contains(out, "dry run: nothing was written") {
		t.Fatalf("code=%d out=%s", code, out)
	}
	if got, _ := os.ReadFile(target); string(got) != userClaude {
		t.Fatalf("dry run modified the harness file: %q", got)
	}
	if countFiles(t, home) != before {
		t.Fatal("dry run created files")
	}
}

func TestSetupApplyIsReversibleIdempotentAndBacksUp(t *testing.T) {
	bin := buildBemeBinary(t, "beme")
	home, cfg, target := setupHome(t, userClaude)
	args := []string{"setup", "--defaults", "--harness", "claude-code", "--config", cfg, "--apply"}
	out, se, code := runSetup(t, bin, home, "", args...)
	if code != 0 {
		t.Fatalf("apply failed: %s %s", out, se)
	}
	got, _ := os.ReadFile(target)
	if !strings.HasPrefix(string(got), userClaude) || !strings.Contains(string(got), "<!-- BEGIN beme-rules:managed -->") ||
		!strings.Contains(string(got), "command output as proof") {
		t.Fatalf("block not installed correctly:\n%s", got)
	}
	backups, _ := filepath.Glob(filepath.Join(cfg, "backups", "CLAUDE.md.pre-setup-*"))
	if len(backups) != 1 {
		t.Fatalf("want 1 backup, got %v", backups)
	}
	if b, _ := os.ReadFile(backups[0]); string(b) != userClaude {
		t.Fatal("backup is not the original")
	}
	if ents, _ := filepath.Glob(filepath.Join(cfg, "rules", "entries", "RULE-*.md")); len(ents) != 6 {
		t.Fatalf("want 6 entries, got %d", len(ents))
	}
	if !strings.Contains(out, "$ beme build") || !strings.Contains(out, "$ beme doctor") || strings.Contains(out, "degraded") {
		t.Fatalf("build/doctor not clean:\n%s", out)
	}
	if !strings.Contains(out, "claude mcp add beme") {
		t.Fatalf("MCP registration not printed:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); err == nil {
		t.Fatal("setup must not edit harness MCP configuration")
	}

	// second apply: same bytes, no extra backup
	if _, _, code := runSetup(t, bin, home, "", args...); code != 0 {
		t.Fatal("second apply failed")
	}
	again, _ := os.ReadFile(target)
	if !bytes.Equal(got, again) {
		t.Fatal("apply is not idempotent")
	}
	if b, _ := filepath.Glob(filepath.Join(cfg, "backups", "CLAUDE.md.pre-setup-*")); len(b) != 1 {
		t.Fatalf("idempotent apply made another backup: %v", b)
	}

	// the rules are queryable through the normal resolver
	pv, _, code := runSetup(t, bin, home, "", "preview", "--task", "working rules about being short and direct and showing command output", "--config", cfg, "--json")
	if code != 0 || !strings.Contains(pv, "rec_rule-001") {
		t.Fatalf("rules not served by preview (code %d): %s", code, pv)
	}

	// remove restores the original bytes exactly and keeps the user's data
	if _, _, code := runSetup(t, bin, home, "", "setup", "--remove", "--harness", "claude-code", "--config", cfg, "--apply"); code != 0 {
		t.Fatal("remove failed")
	}
	if restored, _ := os.ReadFile(target); string(restored) != userClaude {
		t.Fatalf("remove did not restore the original bytes: %q", restored)
	}
	if _, err := os.Stat(filepath.Join(cfg, "rules", "answers.json")); err != nil {
		t.Fatal("remove must keep the user's answers")
	}
}

func TestSetupRefreshRerendersFromSavedAnswers(t *testing.T) {
	bin := buildBemeBinary(t, "beme")
	home, cfg, target := setupHome(t, userClaude)
	base := []string{"--harness", "claude-code", "--config", cfg, "--no-build"}
	if _, _, code := runSetup(t, bin, home, "", append([]string{"setup", "--defaults", "--apply"}, base...)...); code != 0 {
		t.Fatal("apply failed")
	}
	ans := filepath.Join(cfg, "rules", "answers.json")
	var m map[string]any
	data, _ := os.ReadFile(ans)
	_ = json.Unmarshal(data, &m)
	m["review_doc_max_lines"] = 40
	edited, _ := json.Marshal(m)
	if err := os.WriteFile(ans, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runSetup(t, bin, home, "", append([]string{"setup", "--refresh", "--apply"}, base...)...); code != 0 {
		t.Fatal("refresh failed")
	}
	got, _ := os.ReadFile(target)
	if !strings.Contains(string(got), "at most about 40 lines") || strings.Contains(string(got), "about 60 lines") {
		t.Fatalf("refresh did not re-render:\n%s", got)
	}
}

func TestSetupKeepsUserOwnedEntries(t *testing.T) {
	bin := buildBemeBinary(t, "beme")
	home, cfg, _ := setupHome(t, "")
	entries := filepath.Join(cfg, "rules", "entries")
	if err := os.MkdirAll(entries, 0o700); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(entries, "my-scoped-rule.md")
	if err := os.WriteFile(mine, []byte("---\nid: MINE-1\ntitle: \"x\"\ntype: preference\nstatus: active\n---\n\nmine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runSetup(t, bin, home, "", "setup", "--defaults", "--harness", "claude-code", "--config", cfg, "--apply", "--no-build"); code != 0 {
		t.Fatal("apply failed")
	}
	if _, err := os.Stat(mine); err != nil {
		t.Fatal("setup removed a user-owned entry")
	}
}

func TestSetupRefusesWithoutTerminalOrAnswers(t *testing.T) {
	bin := buildBemeBinary(t, "beme")
	home, cfg, target := setupHome(t, userClaude)
	_, se, code := runSetup(t, bin, home, "", "setup", "--harness", "claude-code", "--config", cfg, "--apply")
	if code != 2 || !strings.Contains(se, "--answers") {
		t.Fatalf("code=%d stderr=%s", code, se)
	}
	if got, _ := os.ReadFile(target); string(got) != userClaude {
		t.Fatal("file changed despite refusal")
	}
}

func TestSetupInvalidAnswersWriteNothing(t *testing.T) {
	bin := buildBemeBinary(t, "beme")
	home, cfg, target := setupHome(t, userClaude)
	bad := filepath.Join(home, "bad.json")
	_ = os.WriteFile(bad, []byte(`{"schema_version":1,"style":"short","autonomy":"act","custom_rules":["<!-- END beme-rules:managed -->"]}`), 0o600)
	before := countFiles(t, home)
	_, se, code := runSetup(t, bin, home, "", "setup", "--answers", bad, "--harness", "claude-code", "--config", cfg, "--apply")
	if code != 1 || !strings.Contains(se, "marker") {
		t.Fatalf("code=%d stderr=%s", code, se)
	}
	if got, _ := os.ReadFile(target); string(got) != userClaude || countFiles(t, home) != before {
		t.Fatal("invalid answers changed the filesystem")
	}
}

func TestSetupUnknownHarnessAndPrintQuestions(t *testing.T) {
	bin := buildBemeBinary(t, "beme")
	home, cfg, _ := setupHome(t, "")
	if _, _, code := runSetup(t, bin, home, "", "setup", "--defaults", "--harness", "vim", "--config", cfg); code != 2 {
		t.Fatalf("unknown harness must exit 2, got %d", code)
	}
	out, _, code := runSetup(t, bin, home, "", "setup", "--print-questions")
	var qs []map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &qs) != nil || len(qs) != 8 {
		t.Fatalf("print-questions: code=%d out=%s", code, out)
	}
}

func TestSetupPrintsTextForHarnessesWithoutManagedFile(t *testing.T) {
	bin := buildBemeBinary(t, "beme")
	home, cfg, _ := setupHome(t, "")
	out, _, code := runSetup(t, bin, home, "", "setup", "--defaults", "--harness", "cursor", "--config", cfg, "--apply", "--no-build")
	if code != 0 || !strings.Contains(out, "place this text") {
		t.Fatalf("code=%d out=%s", code, out)
	}
}

func TestInterviewAcceptsDefaultsAndOverrides(t *testing.T) {
	var w bytes.Buffer
	a, err := interview(strings.NewReader("\n\n\n\n\n\n\n\n"), &w)
	if err != nil || a.Style != "short" || a.Autonomy != "act" || a.DocMaxLines != 60 || len(a.Gates) != 3 {
		t.Fatalf("defaults: %+v err=%v", a, err)
	}
	in := "n\nn\nbalanced\nn\nact\npurchase\n0\nPrefer small commits.\n\n"
	a, err = interview(strings.NewReader(in), &w)
	if err != nil || a.Evidence || a.OneRecommendation || a.Style != "balanced" || a.FailureReporting ||
		len(a.Gates) != 1 || a.Gates[0] != "purchase" || a.DocMaxLines != 0 || len(a.Custom) != 1 {
		t.Fatalf("overrides: %+v err=%v", a, err)
	}
	if _, err = interview(strings.NewReader("y\n"), &w); err == nil {
		t.Fatal("truncated input must be an error, not silent defaults")
	}
	a, err = interview(strings.NewReader("\n\n\n\n\n\n\n<!-- x\n\n"), &w)
	if err != nil || len(a.Custom) != 0 {
		t.Fatalf("a forged-marker custom rule must be rejected at the prompt, not stored: %+v err=%v", a, err)
	}
}

func TestSetupDoesNotTouchRealHomeOnWindowsPathsEither(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by the temporary HOME in the other tests")
	}
	// the harness fixtures above already prove HOME isolation; this guards
	// that targetFile follows HOME so a test can never reach a real file.
	t.Setenv("HOME", t.TempDir())
	p, err := targetFile("claude-code")
	if err != nil || !strings.HasPrefix(p, os.Getenv("HOME")) {
		t.Fatalf("targetFile ignores HOME: %s %v", p, err)
	}
}

func TestInterviewReprompts(t *testing.T) {
	// y, y, short, y, act, bad gate then none, 5 (out of range) then 40, bad custom then good, end
	in := "\n\n\n\n\npurchse\nnone\n5\n40\n<!-- x -->\nmy rule\n\n"
	var out bytes.Buffer
	a, err := interview(strings.NewReader(in), &out)
	if err != nil {
		t.Fatalf("interview failed instead of re-prompting: %v\n%s", err, out.String())
	}
	if len(a.Gates) != 0 || a.DocMaxLines != 40 || len(a.Custom) != 1 || a.Custom[0] != "my rule" {
		t.Fatalf("unexpected answers: %+v", a)
	}
}

func TestApplyRulesKeepsOldEntriesWhenStagingFails(t *testing.T) {
	cfg := t.TempDir()
	rules := filepath.Join(cfg, "rules")
	a := setup.Defaults()
	if err := applyRules(cfg, rules, a, setup.Rules(a)); err != nil {
		t.Fatal(err)
	}
	entries := filepath.Join(rules, "entries")
	// A directory where the staging file must go makes staging fail.
	if err := os.Mkdir(filepath.Join(entries, "RULE-002.md.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	b := setup.Defaults()
	b.Style = "balanced"
	if err := applyRules(cfg, rules, b, setup.Rules(b)); err == nil {
		t.Fatal("expected a staging failure")
	}
	list, _ := os.ReadDir(entries)
	n := 0
	for _, e := range list {
		if managedRuleFile.MatchString(e.Name()) {
			n++
		}
	}
	if n != len(setup.Rules(a)) {
		t.Fatalf("previous entries were lost: %d left", n)
	}
}
