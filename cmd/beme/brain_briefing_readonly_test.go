package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// briefing must be a TRUE read: no lock file, no directory creation, no writes
// of any kind — so it works under a read-only sandbox (this is the real
// failure mode a live Codex run hit: the session-start-derived old behavior
// tried to create .lock and failed under `-s read-only`).
func TestBriefingIsReadOnlyUnderARealReadOnlySandbox(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits do not establish Windows ACL behavior")
	}
	if testing.Short() {
		t.Skip("e2e")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not bind root")
	}
	r := newBrainRig(t)
	proj := filepath.Join(r.home, "proj")
	os.MkdirAll(proj, 0o755)
	r.mustRun("", "brain", "init")
	r.mustRun("", "brain", "project", "add", "demo", "--root", proj)
	r.mustRun("", "brain", "capture", "--cwd", proj, "--type", "decision", "--title", "Use SQLite",
		"--body", "Local state lives in a single SQLite file.", "--why", "Zero-ops and offline-first.")
	r.mustRun("", "brain", "doctor") // promote it, so briefing has real content to show

	vault := filepath.Join(r.cfg, "data", "brain")
	// A second, NOT-yet-promoted proposal: proves --doctor's promotion is what
	// gets skipped read-only, not the read of already-live content.
	r.mustRun("", "brain", "capture", "--cwd", proj, "--type", "learning", "--title", "Pending lesson",
		"--body", "This one is still a proposal when the sandbox goes read-only.")

	if err := chmodTree(vault, 0o500); err != nil {
		t.Fatal(err)
	}
	defer chmodTree(vault, 0o700)

	out, se, code := r.run("", "brain", "briefing", "--cwd", proj)
	if code != 0 {
		t.Fatalf("plain briefing must succeed read-only, got %d: %s", code, se)
	}
	if !strings.Contains(out, "Use SQLite") {
		t.Fatalf("already-live content must still show:\n%s", out)
	}
	if fileExistsCLI(filepath.Join(vault, ".lock")) {
		t.Fatal("plain briefing must never create a lock file")
	}

	out2, se2, code2 := r.run("", "brain", "briefing", "--cwd", proj, "--doctor")
	if code2 != 0 {
		t.Fatalf("--doctor must degrade, not fail, read-only: %d: %s", code2, se2)
	}
	if !strings.Contains(out2, "maintenance pass skipped") {
		t.Fatalf("--doctor's failure must be reported as a notice:\n%s", out2)
	}
	if !strings.Contains(out2, "Use SQLite") {
		t.Fatalf("the read must still succeed even when --doctor degrades:\n%s", out2)
	}
	if strings.Contains(out2, "Pending lesson") {
		t.Fatal("a proposal that couldn't be promoted must not appear as if it were live")
	}

	chmodTree(vault, 0o700)
	r.mustRun("", "brain", "briefing", "--cwd", proj, "--doctor") // now succeeds, promotes it
	if out3 := r.mustRun("", "brain", "briefing", "--cwd", proj); !strings.Contains(out3, "Pending lesson") {
		t.Fatalf("once writable again, --doctor must promote it for real:\n%s", out3)
	}
}

func chmodTree(root string, mode os.FileMode) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		return os.Chmod(p, mode)
	})
}

func fileExistsCLI(p string) bool { _, err := os.Stat(p); return err == nil }
