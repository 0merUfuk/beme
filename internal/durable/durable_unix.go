//go:build !windows

package durable

import (
	"errors"
	"os"
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
	defer d.Close()
	if err := d.Sync(); err != nil {
		if errors.Is(err, syscall.ENOTTY) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
			return unix.Fsync(int(d.Fd()))
		}
		return err
	}
	return nil
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
