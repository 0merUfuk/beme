//go:build unix

package brain

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSessionAccountingUnixPermissionRefusalIsNotRetried(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Unix permission bits do not bind root; Windows uses its separate real DACL control")
	}
	v, _ := newVault(t)
	if err := v.RecordSession("demo", t0); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(v.Root, ".state")
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0700); err != nil {
			t.Error(err)
		}
	})
	changes, commits := 0, 0
	start := time.Now()
	err := v.updateState("demo", func(*projState) error { changes++; return nil }, func() error { commits++; return nil })
	var pe *os.PathError
	if !errors.As(err, &pe) || pe.Op != "open" || pe.Path != filepath.Join(dir, "demo.lock") || !errors.Is(err, syscall.EACCES) {
		t.Fatalf("original permanent permission error was lost: %v", err)
	}
	if time.Since(start) >= 500*time.Millisecond {
		t.Fatal("Unix permission refusal was retried as Windows contention")
	}
	if changes != 0 || commits != 0 {
		t.Fatalf("callback without ownership: change=%d commit=%d", changes, commits)
	}
	assertKnownInvocations(t, v, 1)
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := v.RecordSession("demo", t0); err != nil {
		t.Fatal(err)
	}
	assertKnownInvocations(t, v, 2)
}
