// Package durable provides crash-durability, erasure, and inter-process
// locking primitives for Be Me's enforcement state (ADR-027 §5).
//
// Guarantees, per platform documentation:
//   - Unix: fsync on a file does not persist its directory entry; an explicit
//     fsync of the containing directory is required (fsync(2)). Every
//     operation here that creates, renames, or removes a name flushes the
//     parent directory before returning.
//   - Windows: FlushFileBuffers is documented for file handles opened with
//     GENERIC_WRITE (and, with administrative privileges, volume handles); no
//     documented user-mode call flushes a directory entry. Renames use
//     MoveFileExW with MOVEFILE_WRITE_THROUGH, which "does not return until
//     the file is actually moved on the disk". DeleteFileW only marks a file
//     for deletion on close, so the removal of a name is not durable.
//
// Erase therefore zeroizes and flushes file content before unlinking it: on
// every platform a lost unlink can only leave an empty file behind, never the
// erased content. On Windows the empty name itself may reappear after a power
// loss. Storage-level remnants (copy-on-write filesystems, SSD wear levelling,
// snapshots) are out of scope: erasure is file-level.
package durable

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

var (
	// ErrNotRegular: Erase refuses symlinks, directories, and devices.
	ErrNotRegular = errors.New("not a regular file")
	// ErrMultipleLinks: Erase refuses files with other hard links, whose
	// content is shared with names it has not been asked to erase.
	ErrMultipleLinks = errors.New("file has other hard links")
	// ErrLocked: another maintenance operation holds the lock.
	ErrLocked = errors.New("another Be Me maintenance operation is running")
	// ErrNotDirectory: a path that must be a directory is something else.
	ErrNotDirectory = errors.New("path exists and is not a directory")
	// ErrChangedUnderfoot: the directory entry changed between inspection
	// and opening, so the open handle is not the file that was checked.
	ErrChangedUnderfoot = errors.New("file changed between inspection and opening")
)

var (
	hookMu    sync.RWMutex
	flushDir  = syncDir
	flushFile = func(f *os.File) error { return f.Sync() }
)

// SetFlushHooks replaces the directory and file flush functions and returns a
// function restoring the previous ones. It is a verification hook for tests
// that inject flush failures; production code never calls it. A nil argument
// keeps the current function.
func SetFlushHooks(dir func(string) error, file func(*os.File) error) (restore func()) {
	hookMu.Lock()
	prevDir, prevFile := flushDir, flushFile
	if dir != nil {
		flushDir = dir
	}
	if file != nil {
		flushFile = file
	}
	hookMu.Unlock()
	return func() {
		hookMu.Lock()
		flushDir, flushFile = prevDir, prevFile
		hookMu.Unlock()
	}
}

// PlatformDirFlush is the real directory flush for this platform, for hooks
// that wrap it.
func PlatformDirFlush(dir string) error { return syncDir(dir) }

// eraseRaceHook runs between Erase's inspection of a path and its open. It
// is a verification hook: tests use it to replace the entry in exactly the
// window a hostile process would need, so the no-follow open and the
// identity check are exercised rather than assumed. Production never sets it.
var eraseRaceHook func(path string)

// SetEraseRaceHook installs the between-inspect-and-open hook and returns a
// function restoring the previous one.
func SetEraseRaceHook(fn func(path string)) (restore func()) {
	hookMu.Lock()
	prev := eraseRaceHook
	eraseRaceHook = fn
	hookMu.Unlock()
	return func() {
		hookMu.Lock()
		eraseRaceHook = prev
		hookMu.Unlock()
	}
}

func runEraseRaceHook(path string) {
	hookMu.RLock()
	fn := eraseRaceHook
	hookMu.RUnlock()
	if fn != nil {
		fn(path)
	}
}

func doFlushDir(dir string) error {
	hookMu.RLock()
	fn := flushDir
	hookMu.RUnlock()
	return fn(dir)
}

func doFlushFile(f *os.File) error {
	hookMu.RLock()
	fn := flushFile
	hookMu.RUnlock()
	return fn(f)
}

// Exists reports whether path exists. Errors other than not-exist are
// returned: an uninspectable path is never treated as absent.
func Exists(path string) (bool, error) {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	}
	return false, err
}

// SyncDir flushes a directory so creations, renames, and removals inside it
// are durable. A directory that no longer exists has nothing to flush.
func SyncDir(dir string) error {
	ok, err := Exists(dir)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := doFlushDir(dir); err != nil {
		return fmt.Errorf("flush directory %s: %w", dir, err)
	}
	return nil
}

// SyncFile flushes an existing file's content and metadata. A missing file
// has nothing to flush.
func SyncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := doFlushFile(f); err != nil {
		f.Close()
		return fmt.Errorf("flush %s: %w", filepath.Base(path), err)
	}
	return f.Close()
}

// WriteFile atomically replaces path: temp file in the same directory,
// content flush, rename, and a flush of the parent directory.
func WriteFile(path string, data []byte, perm os.FileMode) error {
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
	if err := chmod(tmp, perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := doFlushFile(tmp); err != nil {
		tmp.Close()
		return fmt.Errorf("flush %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := renameReplace(tmpName, path); err != nil {
		return err
	}
	renamed = true
	return SyncDir(dir)
}

// CreateExclusive creates path (never overwriting), flushes it, and flushes
// the parent directory.
func CreateExclusive(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		f.Close()
		// A leftover partial file would make the next exclusive create fail
		// with EEXIST, so a failed cleanup is reported, not discarded.
		if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return errors.Join(err, fmt.Errorf("remove partial %s: %w", filepath.Base(path), rmErr))
		}
		return err
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	if err := doFlushFile(f); err != nil {
		return fail(fmt.Errorf("flush %s: %w", filepath.Base(path), err))
	}
	if err := f.Close(); err != nil {
		return fail(err)
	}
	return SyncDir(filepath.Dir(path))
}

// EnsureDir creates dir if needed and flushes the containing directory of
// every entry it creates, so a crash cannot lose one of them. An existing
// path that is not a directory is an error, never a silent success.
func EnsureDir(dir string) error {
	info, err := os.Lstat(dir)
	switch {
	case err == nil && info.IsDir():
		return nil
	case err == nil:
		return fmt.Errorf("%s: %w", dir, ErrNotDirectory)
	case errors.Is(err, syscall.ENOTDIR):
		// an ancestor exists and is not a directory
		return fmt.Errorf("%s: %w", dir, ErrNotDirectory)
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	// Collect the missing ancestors, outermost first: MkdirAll would create
	// them all at once and leave only the innermost parent flushed.
	missing := []string{}
	for p := filepath.Clean(dir); ; {
		info, err := os.Lstat(p)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s: %w", p, ErrNotDirectory)
			}
			break
		}
		if errors.Is(err, syscall.ENOTDIR) {
			return fmt.Errorf("%s: %w", p, ErrNotDirectory)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		missing = append([]string{p}, missing...)
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	for _, p := range missing {
		if err := os.Mkdir(p, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := SyncDir(filepath.Dir(p)); err != nil {
			return err
		}
	}
	return nil
}

// Remove unlinks path (absent is fine) and flushes the parent directory —
// also when the file was already gone, so a retry completes a flush that an
// earlier attempt could not.
func Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return SyncDir(filepath.Dir(path))
}

// Erase zeroizes and flushes a regular file's content, unlinks it, and
// flushes the parent directory. It reports whether the file existed. An
// absent file still gets its parent flushed (retry completion). Symlinks and
// files with other hard links are refused before anything is modified.
func Erase(path string) (existed bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, SyncDir(filepath.Dir(path))
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return true, fmt.Errorf("erase %s: %w", filepath.Base(path), ErrNotRegular)
	}
	// Open without following links, then confirm the open handle is the very
	// entry Lstat inspected: otherwise a process that swaps the name for a
	// symlink between the two calls could redirect the zeroizing truncate at
	// a file the caller never asked to erase (TOCTOU).
	runEraseRaceHook(path)
	f, err := openForErase(path)
	if err != nil {
		return true, err
	}
	same, err := sameFile(info, f)
	if err != nil {
		f.Close()
		return true, err
	}
	if !same {
		f.Close()
		return true, fmt.Errorf("erase %s: %w", filepath.Base(path), ErrChangedUnderfoot)
	}
	links, err := linkCount(f)
	if err != nil {
		f.Close()
		return true, err
	}
	if links > 1 {
		f.Close()
		return true, fmt.Errorf("erase %s: %w", filepath.Base(path), ErrMultipleLinks)
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return true, err
	}
	if err := doFlushFile(f); err != nil {
		f.Close()
		return true, fmt.Errorf("flush %s: %w", filepath.Base(path), err)
	}
	if err := f.Close(); err != nil {
		return true, err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return true, err
	}
	return true, SyncDir(filepath.Dir(path))
}

// Lock is an exclusive inter-process maintenance lock.
type Lock struct {
	f *os.File
}

// AcquireLock takes the exclusive lock at path, retrying until timeout.
func AcquireLock(path string, timeout time.Duration) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		busy, err := tryLock(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		if !busy {
			return &Lock{f: f}, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, ErrLocked
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Release drops the lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	uerr := unlock(l.f)
	cerr := l.f.Close()
	l.f = nil
	if uerr != nil {
		return uerr
	}
	return cerr
}
