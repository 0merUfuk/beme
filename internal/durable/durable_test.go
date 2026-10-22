package durable_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/durable"
)

func TestWriteFileReplacesAndLeavesNoTemps(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ledger.json")
	for _, v := range []string{"v1", "v2"} {
		if err := durable.WriteFile(p, []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if data, _ := os.ReadFile(p); string(data) != "v2" {
		t.Fatalf("content %q, want v2", data)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(p); info.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestFlushFailuresAreReported(t *testing.T) {
	injected := errors.New("injected flush failure")
	dir := t.TempDir()
	restore := durable.SetFlushHooks(func(string) error { return injected }, nil)
	for name, op := range map[string]func() error{
		"write":  func() error { return durable.WriteFile(filepath.Join(dir, "a"), []byte("x"), 0o600) },
		"create": func() error { return durable.CreateExclusive(filepath.Join(dir, "b"), []byte("x"), 0o600) },
		"mkdir":  func() error { return durable.EnsureDir(filepath.Join(dir, "c")) },
		"remove": func() error { return durable.Remove(filepath.Join(dir, "absent")) },
		"erase":  func() error { _, err := durable.Erase(filepath.Join(dir, "absent")); return err },
	} {
		if err := op(); !errors.Is(err, injected) {
			t.Errorf("%s must surface the directory flush failure; got %v", name, err)
		}
	}
	restore()
	restore = durable.SetFlushHooks(nil, func(*os.File) error { return injected })
	defer restore()
	if err := durable.WriteFile(filepath.Join(dir, "d"), []byte("x"), 0o600); !errors.Is(err, injected) {
		t.Errorf("write must surface the file flush failure; got %v", err)
	}
	p := filepath.Join(dir, "e")
	os.WriteFile(p, []byte("secret"), 0o600)
	if _, err := durable.Erase(p); !errors.Is(err, injected) {
		t.Errorf("erase must surface the file flush failure; got %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Error("erase must not unlink before the zeroized content is flushed")
	}
}

// TestEraseZeroizesBeforeUnlink records the order of operations: content is
// truncated and flushed, the name removed, then the directory flushed — and a
// retry on an already-absent file still flushes the directory.
func TestEraseZeroizesBeforeUnlink(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "trace.json")
	if err := os.WriteFile(p, []byte("purged content"), 0o600); err != nil {
		t.Fatal(err)
	}
	var fileFlushSize int64 = -1
	var dirFlushes []string
	restore := durable.SetFlushHooks(func(d string) error {
		if _, err := os.Stat(p); err == nil {
			t.Error("directory flushed before the name was removed")
		}
		dirFlushes = append(dirFlushes, d)
		return nil
	}, func(f *os.File) error {
		info, _ := f.Stat()
		fileFlushSize = info.Size()
		return f.Sync()
	})
	defer restore()
	existed, err := durable.Erase(p)
	if err != nil || !existed {
		t.Fatalf("erase: existed=%v err=%v", existed, err)
	}
	if fileFlushSize != 0 {
		t.Fatalf("content must be zeroized before the flush; flushed size %d", fileFlushSize)
	}
	if len(dirFlushes) != 1 || dirFlushes[0] != dir {
		t.Fatalf("parent directory must be flushed once; got %v", dirFlushes)
	}
	existed, err = durable.Erase(p)
	if err != nil || existed || len(dirFlushes) != 2 {
		t.Fatalf("retry on an absent file must still flush the parent: existed=%v err=%v flushes=%v", existed, err, dirFlushes)
	}
}

func TestEraseRefusesSymlinksAndHardLinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside.md")
	if err := os.WriteFile(target, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	if _, err := durable.Erase(link); !errors.Is(err, durable.ErrNotRegular) {
		t.Fatalf("symlink must be refused; got %v", err)
	}
	hard := filepath.Join(dir, "hard.md")
	if err := os.Link(target, hard); err != nil {
		t.Skipf("hard links unsupported here: %v", err)
	}
	if _, err := durable.Erase(hard); !errors.Is(err, durable.ErrMultipleLinks) {
		t.Fatalf("hard-linked file must be refused; got %v", err)
	}
	if data, _ := os.ReadFile(target); string(data) != "keep me" {
		t.Fatalf("refused erase modified shared content: %q", data)
	}
}

// TestEraseOpensWithoutFollowingLinks pins the TOCTOU defense: erasing a
// name that is a symlink when it is opened fails without touching the
// target, even though a regular file of the same name would erase fine.
func TestEraseOpensWithoutFollowingLinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	if err := os.WriteFile(target, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	swapped := filepath.Join(dir, "swapped.md")
	if err := os.Symlink(target, swapped); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	if _, err := durable.Erase(swapped); err == nil {
		t.Fatal("erasing through a symlink must fail")
	}
	if data, _ := os.ReadFile(target); string(data) != "keep me" {
		t.Fatalf("symlink target was modified: %q", data)
	}
	// positive control: the same call erases a real file at that name
	if err := os.Remove(swapped); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(swapped, []byte("erase me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if existed, err := durable.Erase(swapped); err != nil || !existed {
		t.Fatalf("a regular file at the same name must erase: existed=%v err=%v", existed, err)
	}
}

// TestEraseSurvivesEntrySwappedAfterInspection drives the actual TOCTOU
// window: the directory entry is replaced between the inspection and the
// open, and every decision must come from the open handle. A symlink
// swapped in must not be followed, and a name that shares its content with
// a file outside the purge must be refused; in both cases that outside file
// keeps its bytes. Inode identity is deliberately not asserted: a
// filesystem may give the replacement the inode just freed, so it is not a
// sound property to rely on.
func TestEraseSurvivesEntrySwappedAfterInspection(t *testing.T) {
	for _, c := range []struct {
		name    string
		swapIn  func(t *testing.T, path, sentinel string)
		wantErr error
	}{
		{"symlink to a file outside the purge", func(t *testing.T, path, sentinel string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(sentinel, path); err != nil {
				t.Skipf("symlinks unsupported here: %v", err)
			}
			// A swapped-in symlink must be refused at the open itself
			// (O_NOFOLLOW / OPEN_REPARSE_POINT), not merely noticed
			// afterwards by the identity check.
		}, durable.ErrNotRegular},
		{"a name sharing its content with a file outside the purge", func(t *testing.T, path, sentinel string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(sentinel, path); err != nil {
				t.Skipf("hard links unsupported here: %v", err)
			}
		}, durable.ErrMultipleLinks},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			sentinel := filepath.Join(dir, "sentinel.md")
			const keep = "bytes that must survive"
			if err := os.WriteFile(sentinel, []byte(keep), 0o600); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(dir, "target.md")
			if err := os.WriteFile(target, []byte("to erase"), 0o600); err != nil {
				t.Fatal(err)
			}
			swapped := false
			restore := durable.SetEraseRaceHook(func(path string) {
				if swapped || path != target {
					return
				}
				swapped = true
				c.swapIn(t, path, sentinel)
			})
			_, err := durable.Erase(target)
			restore()
			if !swapped {
				t.Fatal("fixture: the entry was never swapped")
			}
			if err == nil {
				t.Fatal("erasing an entry that changed after inspection must fail")
			}
			if c.wantErr != nil && !errors.Is(err, c.wantErr) {
				t.Fatalf("error = %v, want %v", err, c.wantErr)
			}
			if data, readErr := os.ReadFile(sentinel); readErr != nil || string(data) != keep {
				t.Fatalf("the file outside the purge was modified: %q %v", data, readErr)
			}
		})
	}
	// positive control: with no swap the same call erases normally.
	dir := t.TempDir()
	p := filepath.Join(dir, "plain.md")
	if err := os.WriteFile(p, []byte("to erase"), 0o600); err != nil {
		t.Fatal(err)
	}
	if existed, err := durable.Erase(p); err != nil || !existed {
		t.Fatalf("unswapped erase must succeed: existed=%v err=%v", existed, err)
	}
}

// TestEraseDoesNotUnlinkAReplacementFile: if the name stops referring to the
// file that was opened and zeroized — a writer replaced it while the purge
// held the handle — the replacement must not be unlinked. POSIX has no
// unlink-this-inode call, so the name is re-checked against the open handle
// immediately before the unlink; this drives exactly that window.
func TestEraseDoesNotUnlinkAReplacementFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	if err := os.WriteFile(target, []byte("to erase"), 0o600); err != nil {
		t.Fatal(err)
	}
	const replacement = "a file someone else created"
	swapped := false
	restore := durable.SetEraseUnlinkHook(func(path string) {
		if swapped || path != target {
			return
		}
		swapped = true
		if err := os.Remove(path); err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(path, []byte(replacement), 0o600); err != nil {
			t.Error(err)
		}
	})
	_, err := durable.Erase(target)
	restore()
	if !swapped {
		t.Fatal("fixture: the entry was never replaced")
	}
	if !errors.Is(err, durable.ErrChangedUnderfoot) {
		t.Fatalf("erase must refuse to unlink a replacement; got %v", err)
	}
	if data, readErr := os.ReadFile(target); readErr != nil || string(data) != replacement {
		t.Fatalf("the replacement file was unlinked or modified: %q %v", data, readErr)
	}
	// positive control: with no replacement the same call erases the file.
	plain := filepath.Join(dir, "plain.md")
	if err := os.WriteFile(plain, []byte("to erase"), 0o600); err != nil {
		t.Fatal(err)
	}
	if existed, err := durable.Erase(plain); err != nil || !existed {
		t.Fatalf("unreplaced erase must succeed: existed=%v err=%v", existed, err)
	}
	if _, err := os.Stat(plain); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unreplaced erase left the file behind")
	}
}

func TestEnsureDirRefusesNonDirectoryAndFlushesEveryLevel(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := durable.EnsureDir(file); !errors.Is(err, durable.ErrNotDirectory) {
		t.Fatalf("an existing non-directory must be refused; got %v", err)
	}
	if err := durable.EnsureDir(filepath.Join(file, "under")); !errors.Is(err, durable.ErrNotDirectory) {
		t.Fatalf("a non-directory ancestor must be refused; got %v", err)
	}
	var flushed []string
	restore := durable.SetFlushHooks(func(d string) error {
		flushed = append(flushed, d)
		return durable.PlatformDirFlush(d)
	}, nil)
	defer restore()
	deep := filepath.Join(dir, "a", "b", "c")
	if err := durable.EnsureDir(deep); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{dir, filepath.Join(dir, "a"), filepath.Join(dir, "a", "b")} {
		found := false
		for _, d := range flushed {
			found = found || d == want
		}
		if !found {
			t.Errorf("containing directory %s of a newly created entry was not flushed (flushed: %v)", want, flushed)
		}
	}
}

func TestSyncDirOnRealDirectory(t *testing.T) {
	if err := durable.PlatformDirFlush(t.TempDir()); err != nil {
		t.Fatalf("directory flush must work on this platform: %v", err)
	}
}

// TestLockSerializesHolders: concurrent holders never overlap, and a held
// lock times out other acquirers with ErrLocked.
func TestLockSerializesHolders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger", ".lock")
	var inside, maxInside int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, err := durable.AcquireLock(path, 10*time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			n := atomic.AddInt32(&inside, 1)
			for {
				m := atomic.LoadInt32(&maxInside)
				if n <= m || atomic.CompareAndSwapInt32(&maxInside, m, n) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
			atomic.AddInt32(&inside, -1)
			if err := l.Release(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if maxInside != 1 {
		t.Fatalf("lock holders overlapped: max %d concurrent", maxInside)
	}
	held, err := durable.AcquireLock(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	if _, err := durable.AcquireLock(path, 50*time.Millisecond); !errors.Is(err, durable.ErrLocked) {
		t.Fatalf("a held lock must time out other acquirers with ErrLocked; got %v", err)
	}
}
