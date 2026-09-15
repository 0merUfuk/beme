//go:build windows

package app

import (
	"golang.org/x/sys/windows"
)

// renameDurable replaces to with from and does not return until the move is
// flushed to disk (MOVEFILE_WRITE_THROUGH).
func renameDurable(from, to string) error {
	f, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	t, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(f, t, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

// syncDir is a no-op on Windows: there is no directory fsync. Renames are
// flushed by MOVEFILE_WRITE_THROUGH and new files by FlushFileBuffers on the
// file handle (os.File.Sync).
func syncDir(string) error { return nil }
