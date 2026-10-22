package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTemplateCannotModifyImmutableBundle(t *testing.T) {
	bundle, _ := syntheticGrades(t)
	target := filepath.Join(bundle, "template.json")
	code, _, raw := runJSON(t, "grade-template", "--bundle", bundle, "--out", target, "--evaluator", "reviewer")
	if code != 2 {
		t.Fatalf("template modified input bundle: %d %s", code, raw)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("refused template must not exist")
	}
}
func TestArtifactSymlinkCannotBypassRepositoryBoundary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation privileges vary")
	}
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	link := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(filepath.Join(root, "docs"), link); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(link, "eval-symlink-test-artifacts")
	defer os.Remove(filepath.Join(root, "docs", "eval-symlink-test-artifacts"))
	if _, err := resolveOut(target); err == nil {
		t.Fatal("symlink bypasses private artifact repository boundary")
	}
}
