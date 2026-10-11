package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const userSettings = `{
  "model": "opus",
  "bigNumber": 12345678901234567890,
  "permissions": {"allow": ["Bash(git status)"]},
  "hooks": {
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "/x/guard.sh", "timeout": 10}]}],
    "SessionStart": [{"hooks": [{"type": "command", "command": "/x/other-start.sh"}]}]
  }
}`

func TestInstallSettingsHooksPreservesEverythingElse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(userSettings), 0o600)
	frag := hooksFragment("/opt/beme/bin/beme", " --config /cfg", false)

	msg, err := installSettingsHooks(path, frag, false, false)
	if err != nil || !strings.Contains(msg, "installed") || !strings.Contains(msg, ".beme-bak") {
		t.Fatalf("install: %q %v", msg, err)
	}
	got := readSettings(t, path)
	if got["model"] != "opus" || !strings.Contains(string(mustMarshal(got)), "12345678901234567890") {
		t.Fatalf("unrelated settings (incl. big numbers) must survive: %s", mustMarshal(got))
	}
	hooks := got["hooks"].(map[string]any)
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Fatal("someone else's PreToolUse hook must survive")
	}
	if !strings.Contains(string(mustMarshal(hooks["SessionStart"])), "other-start.sh") {
		t.Fatal("someone else's SessionStart hook must survive next to ours")
	}
	for _, ev := range []string{"SessionStart", "PreCompact", "SessionEnd"} {
		if !strings.Contains(string(mustMarshal(hooks[ev])), "/opt/beme/bin/beme brain") {
			t.Errorf("%s hook missing", ev)
		}
	}
	// the original is preserved byte-for-byte
	if bak, _ := os.ReadFile(path + ".beme-bak"); string(bak) != userSettings {
		t.Fatal("backup must be the untouched original")
	}

	// idempotent: second install changes nothing and does not rewrite
	before, _ := os.ReadFile(path)
	msg, err = installSettingsHooks(path, frag, false, false)
	after, _ := os.ReadFile(path)
	if err != nil || !strings.Contains(msg, "already up to date") || string(before) != string(after) {
		t.Fatalf("second install must be a no-op: %q %v", msg, err)
	}
	// re-install with a new binary path replaces (not duplicates) our hooks
	if _, err := installSettingsHooks(path, hooksFragment("/new/beme", " --config /cfg", false), false, false); err != nil {
		t.Fatal(err)
	}
	txt := string(mustMarshal(readSettings(t, path)))
	if strings.Contains(txt, "/opt/beme/bin/beme") || strings.Count(txt, "/new/beme brain session-start") != 1 {
		t.Fatalf("reinstall must replace, not duplicate: %s", txt)
	}

	// remove: only ours go; the user's hooks and settings remain
	if _, err := installSettingsHooks(path, nil, true, false); err != nil {
		t.Fatal(err)
	}
	txt = string(mustMarshal(readSettings(t, path)))
	if strings.Contains(txt, "brain") || !strings.Contains(txt, "guard.sh") || !strings.Contains(txt, "other-start.sh") || !strings.Contains(txt, `"model":"opus"`) {
		t.Fatalf("remove must leave everyone else's config: %s", txt)
	}
}

func TestInstallSettingsHooksEdgeCases(t *testing.T) {
	dir := t.TempDir()
	frag := hooksFragment("beme", "", false)

	// missing file → created
	p := filepath.Join(dir, "sub", "settings.json")
	if _, err := installSettingsHooks(p, frag, false, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mustMarshal(readSettings(t, p))), "session-start") {
		t.Fatal("hooks not created in a new file")
	}
	// invalid JSON → refused, file untouched
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{ not json"), 0o600)
	if _, err := installSettingsHooks(bad, frag, false, false); err == nil {
		t.Fatal("invalid JSON must be refused")
	}
	if b, _ := os.ReadFile(bad); string(b) != "{ not json" {
		t.Fatal("refused file must be untouched")
	}
	// dry run → nothing written
	dry := filepath.Join(dir, "dry.json")
	os.WriteFile(dry, []byte(`{"a":1}`), 0o600)
	msg, err := installSettingsHooks(dry, frag, false, true)
	if err != nil || !strings.Contains(msg, "would write") {
		t.Fatalf("dry-run: %q %v", msg, err)
	}
	if b, _ := os.ReadFile(dry); string(b) != `{"a":1}` {
		t.Fatal("dry-run wrote the file")
	}
	// removing from a file that never had them is a no-op
	if msg, _ := installSettingsHooks(dry, nil, true, false); !strings.Contains(msg, "already up to date") {
		t.Fatalf("remove on clean file: %q", msg)
	}
	// paths with spaces are quoted so the shell command stays one command
	sp := hooksFragment("/opt/x/My Tools/beme", " --config '/a b'", false)
	if !strings.Contains(string(mustMarshal(sp)), "'/opt/x/My Tools/beme' brain session-start") {
		t.Fatalf("binary path must be shell-quoted: %s", mustMarshal(sp))
	}
}

func TestBrainAdapterBlockCoexistsWithV1BlockAndIsByteExact(t *testing.T) {
	target := filepath.Join(t.TempDir(), "AGENTS.md")
	seed := "# My own AGENTS\n\nDo great work.\n"
	os.WriteFile(target, []byte(seed), 0o644)

	if err := installBlock(target); err != nil { // V1 block first
		t.Fatal(err)
	}
	withV1, _ := os.ReadFile(target)
	if err := installManaged(brainMarkers, brainInstructions(""), target); err != nil {
		t.Fatal(err)
	}
	both, _ := os.ReadFile(target)
	if strings.Count(string(both), blockBegin) != 1 || strings.Count(string(both), brainMarkers.begin) != 1 {
		t.Fatalf("each block exactly once:\n%s", both)
	}
	if ok, _ := verifyManaged(brainMarkers, brainInstructions(""), target); !ok {
		t.Fatal("brain block must verify")
	}
	if ok, _ := verifyBlock(target); !ok {
		t.Fatal("V1 block must still verify")
	}
	// idempotent
	if err := installManaged(brainMarkers, brainInstructions(""), target); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(target)
	if string(again) != string(both) {
		t.Fatal("second install must not change bytes")
	}
	// removing the brain block restores the file with only the V1 block, byte-exact
	if err := removeManaged(brainMarkers, target); err != nil {
		t.Fatal(err)
	}
	afterRemove, _ := os.ReadFile(target)
	if string(afterRemove) != string(withV1) {
		t.Fatalf("removal must be the exact inverse of install:\nwant %q\ngot  %q", withV1, afterRemove)
	}
	if err := removeBlock(target); err != nil {
		t.Fatal(err)
	}
	if final, _ := os.ReadFile(target); string(final) != seed {
		t.Fatalf("user content must be byte-identical after removing both: %q", final)
	}
}

func readSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := decodeSettings(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
