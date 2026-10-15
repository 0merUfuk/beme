//go:build unix

package brain

import (
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// These node types are Unix filesystem semantics, not portable Windows pipe
// names. The regular/directory/symlink/readonly controls run on every platform.
func TestCaptureAccountingUnreadableIntentStaysUnknown(t *testing.T) {
	v, _ := newVault(t)
	path := v.captureIntentPath("demo", 0)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("retained incomplete intent"), 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0600) })
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Mode 0000 refuses reading for an ordinary Unix user. Root can read it,
	// but occupancy must still be conservative; neither case is skipped.
	mustCapture(t, v, sqliteDecision, t0)
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("unreadable intent changed", err)
	}
	if !v.loadState("demo").AccountingUnknown || len(v.Status().AccountingUnknown) != 1 {
		t.Fatal("unreadable intent was acknowledged")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "retained incomplete intent" {
		t.Fatal("unreadable intent contents changed", err)
	}
}

func TestCaptureAccountingSpecialNodesStayUnknown(t *testing.T) {
	for _, kind := range []string{"fifo", "socket"} {
		t.Run(kind, func(t *testing.T) {
			v, _ := newVault(t)
			path := v.captureIntentPath("demo", 0)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if kind == "fifo" {
				if err := syscall.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				// Unix socket addresses have a short native limit. Relative bind
				// avoids depending on the length of the runner's temporary root.
				t.Chdir(v.Root)
				listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(".state", filepath.Base(path)), Net: "unix"})
				if err != nil {
					t.Fatal(err)
				}
				listener.SetUnlinkOnClose(false)
				t.Cleanup(func() { listener.Close() })
			}
			assertRefusedIntentPreserved(t, v, path)
		})
	}
}
