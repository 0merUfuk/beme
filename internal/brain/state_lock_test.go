package brain

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionAccountingCallbackErrorsAreNotRetried(t *testing.T) {
	for _, stage := range []string{"change", "committed"} {
		t.Run(stage, func(t *testing.T) {
			v, _ := newVault(t)
			if err := v.RecordSession("demo", t0); err != nil {
				t.Fatal(err)
			}
			original := &os.PathError{Op: "open", Path: "synthetic-callback", Err: os.ErrPermission}
			changes, commits := 0, 0
			err := v.updateState("demo", func(st *projState) error {
				changes++
				st.InvocationsSinceCapture++
				if stage == "change" {
					return original
				}
				return nil
			}, func() error { commits++; return original })
			want, wantCommits := 1, 0
			if stage == "committed" {
				want, wantCommits = 2, 1
			}
			if err != original || changes != 1 || commits != wantCommits {
				t.Fatalf("callback error was lost/retried: err=%v changes=%d commits=%d", err, changes, commits)
			}
			assertKnownInvocations(t, v, want)
		})
	}
}

func TestSessionAccountingRefusedLockNodesPreserved(t *testing.T) {
	for _, kind := range []string{"directory", "nonempty-directory", "symlink", "dangling-symlink"} {
		t.Run(kind, func(t *testing.T) {
			v, _ := newVault(t)
			lock := filepath.Join(v.Root, ".state", "demo.lock")
			if err := os.MkdirAll(filepath.Dir(lock), 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "retained-target")
			if kind != "dangling-symlink" {
				if err := os.WriteFile(target, []byte("foreign node must survive"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(target, t0, t0); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "directory", "nonempty-directory":
				if err := os.Mkdir(lock, 0700); err != nil {
					t.Fatal(err)
				}
				if kind == "nonempty-directory" {
					target = filepath.Join(lock, "retained")
					if err := os.WriteFile(target, []byte("foreign node must survive"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Chtimes(lock, t0, t0); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.Symlink(target, lock); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(lock)
			if err != nil {
				t.Fatal(err)
			}
			changes, commits := 0, 0
			start := time.Now()
			err = v.updateState("demo", func(*projState) error { changes++; return nil }, func() error { commits++; return nil })
			if err == nil || changes != 0 || commits != 0 {
				t.Fatalf("foreign node granted ownership: change=%d commit=%d error=%v", changes, commits, err)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("refused-node acquisition exceeded its bounded wait")
			}
			after, err := os.Lstat(lock)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("refused lock node was removed or changed:", err)
			}
			if kind == "dangling-symlink" {
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatal("dangling lock target was created:", err)
				}
			} else if data, err := os.ReadFile(target); err != nil || string(data) != "foreign node must survive" {
				t.Fatal("refused node/target bytes changed:", err)
			}
			assertKnownInvocations(t, v, 0)
		})
	}
}
