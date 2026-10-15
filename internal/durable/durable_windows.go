//go:build windows

package durable

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// renameReplace moves from over to and does not return until the move is on
// disk (MOVEFILE_WRITE_THROUGH).
func renameReplace(from, to string) error {
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

// chmod: Windows has no POSIX permission bits beyond read-only.
func chmod(*os.File, os.FileMode) error { return nil }

// syncDir is a no-op: Windows documents no user-mode directory flush. See the
// package documentation for the resulting limitation.
func syncDir(string) error { return nil }

// openForErase opens the directory entry itself rather than a link target
// (FILE_FLAG_OPEN_REPARSE_POINT), so a symlink swapped in after inspection
// cannot redirect the zeroizing truncate.
func openForErase(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

// handleIsRegular reports whether the OPEN handle refers to a plain file —
// not a reparse point (a symlink or junction swapped in after inspection)
// and not a directory.
func handleIsRegular(f *os.File) (bool, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return false, err
	}
	return info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 &&
		info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0, nil
}

// nameRefersTo reports whether path still names the open file (same volume
// and file index), so a replacement created after the open is not unlinked
// in its place.
func nameRefersTo(path string, f *os.File) (bool, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	h, err := windows.CreateFile(p, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return false, nil
		}
		return false, &os.PathError{Op: "open", Path: path, Err: err}
	}
	defer windows.CloseHandle(h)
	var onDisk, opened windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &onDisk); err != nil {
		return false, err
	}
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &opened); err != nil {
		return false, err
	}
	return onDisk.VolumeSerialNumber == opened.VolumeSerialNumber &&
		onDisk.FileIndexHigh == opened.FileIndexHigh &&
		onDisk.FileIndexLow == opened.FileIndexLow, nil
}

func linkCount(f *os.File) (uint64, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return 0, err
	}
	return uint64(info.NumberOfLinks), nil
}

func tryLock(f *os.File) (busy bool, err error) {
	var ol windows.Overlapped
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return true, nil
	}
	return false, err
}

func unlock(f *os.File) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol)
}
