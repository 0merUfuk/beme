package app

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
)

func TestWriteFileDurableReplacesAndLeavesNoTemps(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ledger.json")
	for _, v := range []string{"v1", "v2"} {
		if err := writeFileDurable(p, []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if data, _ := os.ReadFile(p); string(data) != "v2" {
		t.Fatalf("content %q, want v2", data)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(p); info.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestDurableWritesReportDirectoryFlushFailure(t *testing.T) {
	injected := errors.New("injected directory flush failure")
	orig := syncDirHook
	syncDirHook = func(string) error { return injected }
	t.Cleanup(func() { syncDirHook = orig })
	dir := t.TempDir()
	if err := writeFileDurable(filepath.Join(dir, "a"), []byte("x"), 0o600); !errors.Is(err, injected) {
		t.Fatalf("writeFileDurable must surface the flush failure; got %v", err)
	}
	if err := createFileDurable(filepath.Join(dir, "b"), []byte("x"), 0o600); !errors.Is(err, injected) {
		t.Fatalf("createFileDurable must surface the flush failure; got %v", err)
	}
	if err := ensureDirDurable(filepath.Join(dir, "c")); !errors.Is(err, injected) {
		t.Fatalf("ensureDirDurable must surface the flush failure; got %v", err)
	}
}

func TestSyncDirOnRealDirectory(t *testing.T) {
	if err := syncDir(t.TempDir()); err != nil {
		t.Fatalf("directory flush must work on this platform: %v", err)
	}
}

func internalPurgeFixture(t *testing.T) (*Runtime, string) {
	t.Helper()
	base := t.TempDir()
	cfg := filepath.Join(base, "cfg")
	src := filepath.Join(base, "src", "entries", "CAN-001.md")
	for _, d := range []string{filepath.Join(cfg, "sources"), filepath.Dir(src)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(src, []byte("---\nid: CAN-001\ntitle: \"Canary\"\ntype: preference\nstatus: active\n---\n\nDurability boundary canary statement.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	desc := "schema_version: \"1\"\nsource_id: dur-src\ntype: directory\nroot: " + filepath.ToSlash(filepath.Join(base, "src")) +
		"\npurpose: [reusable_knowledge]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: default\nsensitivity: personal_private\nprofiles_allowed: [personal]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"
	if err := os.WriteFile(filepath.Join(cfg, "sources", "src.yaml"), []byte(desc), 0o600); err != nil {
		t.Fatal(err)
	}
	rt, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	return rt, src
}

func storeHolds(t *testing.T, rt *Runtime, needle string) bool {
	t.Helper()
	for _, suffix := range []string{"", "-wal"} {
		if data, err := os.ReadFile(rt.ProjectionPath(contracts.ProfilePersonal) + suffix); err == nil && strings.Contains(string(data), needle) {
			return true
		}
	}
	return false
}

// TestPurgeErasesNothingBeforeDurabilityBoundary: if the anti-resurrection
// ledger or the resumable journal cannot be flushed, the purge aborts before
// touching any store, trace, observation, or canonical file.
func TestPurgeErasesNothingBeforeDurabilityBoundary(t *testing.T) {
	const needle = "Durability boundary canary statement"
	for _, c := range []struct {
		name   string
		failIn func(rt *Runtime) string
	}{
		{"ledger", func(rt *Runtime) string { return rt.ledgerDir() }},
		{"journal", func(rt *Runtime) string { return rt.pendingDir() }},
	} {
		t.Run(c.name, func(t *testing.T) {
			rt, src := internalPurgeFixture(t)
			injected := errors.New("injected flush failure")
			target := c.failIn(rt)
			orig := syncDirHook
			syncDirHook = func(dir string) error {
				if dir == target {
					return injected
				}
				return orig(dir)
			}
			_, err := rt.PhysicalPurge(PurgeRequest{Key: "rec_can-001", Confirm: "rec_can-001", RemoveCanonical: true})
			syncDirHook = orig
			if !errors.Is(err, injected) {
				t.Fatalf("purge must stop at the %s durability boundary; got %v", c.name, err)
			}
			if !storeHolds(t, rt, needle) {
				t.Fatalf("erasure began before the %s reached stable storage", c.name)
			}
			if _, err := os.Stat(src); err != nil {
				t.Fatalf("canonical file removed before the %s reached stable storage", c.name)
			}
			if _, err := rt.PhysicalPurge(PurgeRequest{Key: "rec_can-001", Confirm: "rec_can-001", RemoveCanonical: true}); err != nil {
				t.Fatalf("purge must complete once flushes succeed: %v", err)
			}
			if storeHolds(t, rt, needle) {
				t.Fatal("completed purge left content in the store")
			}
		})
	}
}

func TestMergeIgnoreRules(t *testing.T) {
	cases := []struct {
		name, existing, want string
		changed              bool
	}{
		{"empty", "", "purge.key\npending/\n", true},
		{"unrelated rules without trailing newline", "custom-rule\n*.bak", "custom-rule\n*.bak\npurge.key\npending/\n", true},
		{"anchored equivalents present", "/purge.key\npending\n", "/purge.key\npending\n", false},
		{"partial", "# keep\npurge.key\n", "# keep\npurge.key\npending/\n", true},
		{"crlf file", "custom\r\n", "custom\r\npurge.key\npending/\n", true},
		{"commented rule is not a rule", "# purge.key\n", "# purge.key\npurge.key\npending/\n", true},
	}
	for _, c := range cases {
		got, changed := mergeIgnoreRules(c.existing, ledgerIgnoreRules)
		if got != c.want || changed != c.changed {
			t.Errorf("%s: got %q changed=%v, want %q changed=%v", c.name, got, changed, c.want, c.changed)
		}
		if again, changed2 := mergeIgnoreRules(got, ledgerIgnoreRules); again != got || changed2 {
			t.Errorf("%s: merge is not idempotent: %q", c.name, again)
		}
	}
}

// TestExistingLedgerGitignoreGainsRules: a pre-existing ledger/.gitignore
// keeps its rules byte for byte and gains the key and journal rules once.
func TestExistingLedgerGitignoreGainsRules(t *testing.T) {
	rt, _ := internalPurgeFixture(t)
	if err := os.MkdirAll(rt.ledgerDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	ignore := filepath.Join(rt.ledgerDir(), ".gitignore")
	if err := os.WriteFile(ignore, []byte("custom-rule\n*.bak"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.PhysicalPurge(PurgeRequest{Key: "rec_can-001", Confirm: "rec_can-001"}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Forget(contracts.ProfilePersonal, "rec_other", "second ledger write"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ignore)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "custom-rule\n*.bak\npurge.key\npending/\n" {
		t.Fatalf(".gitignore = %q", data)
	}
}
