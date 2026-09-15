package app

// Durable file replacement for the tombstone ledger, the purge key, and the
// purge journal (ADR-027 durability boundary).
//
// Guarantee: when writeFileDurable returns nil, the new content and the
// directory entry naming it were handed to stable storage with explicit
// flushes — the file is fsynced (os.File.Sync; F_FULLFSYNC on macOS), then:
//   - Unix: rename(2), then fsync of the parent directory;
//   - Windows: MoveFileExW(MOVEFILE_REPLACE_EXISTING|MOVEFILE_WRITE_THROUGH),
//     which returns only after the move is flushed (Windows offers no
//     directory fsync; write-through is the documented equivalent).
//
// It cannot defeat storage that acknowledges flushes it does not perform
// (volatile drive caches, some network or virtualized filesystems). Any
// flush error is returned, and purge aborts before erasing anything.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// syncDirHook flushes a directory; internal tests replace it to inject
// durability failures.
var syncDirHook = syncDir

func writeFileDurable(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			os.Remove(tmpName)
		}
	}()
	if runtime.GOOS != "windows" {
		if err := tmp.Chmod(perm); err != nil {
			tmp.Close()
			return err
		}
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("flush %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := renameDurable(tmpName, path); err != nil {
		return err
	}
	renamed = true
	return syncDirHook(dir)
}

// createFileDurable creates path exclusively (never overwriting) and flushes
// the file and its directory entry.
func createFileDurable(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		f.Close()
		os.Remove(path)
		return err
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(fmt.Errorf("flush %s: %w", filepath.Base(path), err))
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	return syncDirHook(filepath.Dir(path))
}

// removeFileDurable removes path (absent is fine) and flushes the directory.
func removeFileDurable(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirHook(filepath.Dir(path))
}

// ensureDirDurable creates dir if needed and flushes the parent directory so
// the new entry survives a crash.
func ensureDirDurable(dir string) error {
	ok, err := statExists(dir)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return syncDirHook(filepath.Dir(dir))
}

// statExists reports whether path exists. Errors other than not-exist are
// returned: an uninspectable path is never treated as absent.
func statExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	}
	return false, err
}
