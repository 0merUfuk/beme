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
	if err := os.Symlink(target, link); err == nil {
		if _, err := durable.Erase(link); !errors.Is(err, durable.ErrNotRegular) {
			t.Fatalf("symlink must be refused; got %v", err)
		}
	}
	hard := filepath.Join(dir, "hard.md")
	if err := os.Link(target, hard); err == nil {
		if _, err := durable.Erase(hard); !errors.Is(err, durable.ErrMultipleLinks) {
			t.Fatalf("hard-linked file must be refused; got %v", err)
		}
	}
	if data, _ := os.ReadFile(target); string(data) != "keep me" {
		t.Fatalf("refused erase modified shared content: %q", data)
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
