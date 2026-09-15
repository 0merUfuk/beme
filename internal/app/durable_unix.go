//go:build !windows

package app

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func renameDurable(from, to string) error { return os.Rename(from, to) }

// syncDir fsyncs a directory so a rename or create inside it is durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		// Some platforms reject F_FULLFSYNC on directories; fall back to a
		// plain fsync, which is the POSIX directory-durability primitive.
		if errors.Is(err, syscall.ENOTTY) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
			if ferr := syscall.Fsync(int(d.Fd())); ferr == nil {
				return nil
			}
		}
		return fmt.Errorf("flush directory %s: %w", dir, err)
	}
	return nil
}
