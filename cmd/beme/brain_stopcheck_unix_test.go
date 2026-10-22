//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A FIFO would block a naive open forever; stop-check must refuse it from a
// stat and exit 0 silently.
func TestStopCheckRefusesFIFOTranscript(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	r := newBrainRig(t)
	proj := filepath.Join(r.home, "proj")
	os.MkdirAll(proj, 0o755)
	r.mustRun("", "brain", "init")
	r.mustRun("", "brain", "project", "add", "demo", "--root", proj)
	fifo := filepath.Join(r.home, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"cwd": proj, "session_id": "f", "transcript_path": fifo})
	if out, _, code := r.run(string(b), "brain", "stop-check", "--format", "claude-hook"); code != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("a FIFO transcript must be refused without blocking: code=%d out=%q", code, out)
	}
}
