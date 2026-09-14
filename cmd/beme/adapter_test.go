package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAdapterInstallRemoveIdempotent (FR-043): install twice = one block;
// remove preserves unrelated content.
func TestAdapterInstallRemoveIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "AGENTS.md")

	// unrelated user content must survive
	seed := "# My own AGENTS\n\nDo great work.\n\n" + blockBegin + "\nstale old block\n" + blockEnd + "\n\nMore user content.\n"
	os.WriteFile(target, []byte(seed), 0o644)

	// install replaces the stale managed block, keeps the rest
	if err := installBlock(target); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	txt := string(data)
	if strings.Count(txt, blockBegin) != 1 {
		t.Fatalf("exactly one managed block allowed; found %d", strings.Count(txt, blockBegin))
	}
	if !strings.Contains(txt, "Do great work.") || !strings.Contains(txt, "More user content.") {
		t.Fatal("unrelated user content must be preserved")
	}
	if strings.Contains(txt, "stale old block") {
		t.Fatal("stale managed content must be replaced")
	}
	if !strings.Contains(txt, "resolve relevant Be Me context") {
		t.Fatal("canonical bootstrap text missing")
	}

	// second install: idempotent
	first, _ := os.ReadFile(target)
	if err := installBlock(target); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(target)
	if string(first) != string(second) {
		t.Fatal("second install must be a no-op (idempotent)")
	}

	// verify matches canonical
	ok, err := verifyBlock(target)
	if err != nil || !ok {
		t.Fatalf("verify must pass after install: ok=%v err=%v", ok, err)
	}

	// remove leaves the user content intact and no markers
	if err := removeBlock(target); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(target)
	txt = string(data)
	if strings.Contains(txt, blockBegin) || strings.Contains(txt, blockEnd) {
		t.Fatal("markers must be gone after remove")
	}
	if !strings.Contains(txt, "Do great work.") || !strings.Contains(txt, "More user content.") {
		t.Fatal("remove must preserve unrelated user content")
	}

	// remove again: no-op, no error
	if err := removeBlock(target); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterUnsupportedHarness(t *testing.T) {
	if _, err := targetFile("vscode"); err == nil {
		t.Fatal("unsupported harness must error (v1: codex, claude-code)")
	}
}
