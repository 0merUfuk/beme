//go:build !windows

package durable

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func renameReplace(from, to string) error { return os.Rename(from, to) }

func chmod(f *os.File, perm os.FileMode) error { return f.Chmod(perm) }

// syncDir fsyncs a directory (fsync(2): required to persist directory
// entries). Platforms that reject F_FULLFSYNC on directories fall back to a
// plain fsync.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		if errors.Is(err, syscall.ENOTTY) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
			err = unix.Fsync(int(d.Fd()))
		}
		if err != nil {
			d.Close()
			return err
		}
	}
	// A Close error can report a deferred write failure, so it is not
	// discarded: the flush is only complete once the handle closes cleanly.
	return d.Close()
}

// openForErase opens a regular file for writing without following a final
// symlink (O_NOFOLLOW), so the name cannot be redirected after inspection.
func openForErase(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.EMLINK) {
			return nil, fmt.Errorf("erase %s: %w", filepath.Base(path), ErrNotRegular)
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

// handleIsRegular reports whether the OPEN handle refers to a regular file,
// decided after the no-follow open rather than from an earlier Lstat.
func handleIsRegular(f *os.File) (bool, error) {
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func linkCount(f *os.File) (uint64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 1, nil
	}
	return uint64(st.Nlink), nil
}

func tryLock(f *os.File) (busy bool, err error) {
	err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return true, nil
	}
	return false, err
}

func unlock(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }
