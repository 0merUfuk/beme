package brain

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Explicit classic disposition keeps the lock delete-pending until h closes.
// DeleteFile may instead use POSIX deletion and free the name while h is open.
// This is the native CreateFile boundary, not a mock of RecordSession's result.
func TestSessionAccountingWindowsDeletePendingRecovers(t *testing.T) {
	v, _ := newVault(t)
	lock := filepath.Join(v.Root, ".state", "demo.lock")
	if err := os.MkdirAll(filepath.Dir(lock), 0700); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(lock)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if h != windows.InvalidHandle {
			if err := windows.CloseHandle(h); err != nil {
				t.Error("close synthetic delete-pending handle:", err)
			}
		}
	}()
	// FILE_DISPOSITION_INFO contains one BOOLEAN byte, not a Win32 BOOL.
	// Use FileDispositionInfo, not FileDispositionInfoEx with POSIX semantics.
	deleteFile := byte(1)
	if err := windows.SetFileInformationByHandle(h, windows.FileDispositionInfo, &deleteFile, 1); err != nil {
		t.Fatal("set classic delete-pending disposition:", err)
	}
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if f != nil {
		f.Close()
	}
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("fixture did not establish delete-pending access denial: %v", err)
	}
	t.Logf("native classic FileDispositionInfo exclusive create: %v", err)

	var changes, commits atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- v.updateState("demo", func(st *projState) error {
			changes.Add(1)
			st.InvocationsSinceCapture++
			return nil
		}, func() error { commits.Add(1); return nil })
	}()
	var early error
	finished := false
	select {
	case early = <-done:
		finished = true
	case <-time.After(50 * time.Millisecond):
	}
	changesBefore, commitsBefore := changes.Load(), commits.Load()
	closeErr := windows.CloseHandle(h)
	if closeErr == nil {
		h = windows.InvalidHandle
	}
	if !finished {
		select {
		case early = <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("accounting did not finish after releasing delete-pending handle: close=%v", closeErr)
		}
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if finished || changesBefore != 0 || commitsBefore != 0 {
		t.Fatalf("must wait for acquired ownership, not refuse/enter during delete-pending: finished=%t change=%d commit=%d error=%v", finished, changesBefore, commitsBefore, early)
	}
	if early != nil || changes.Load() != 1 || commits.Load() != 1 {
		t.Fatalf("released delete-pending lock did not commit once: change=%d commit=%d error=%v", changes.Load(), commits.Load(), early)
	}
	assertKnownInvocations(t, v, 1)
}

func TestSessionAccountingWindowsPermanentACLRefusal(t *testing.T) {
	v, _ := newVault(t)
	if err := v.RecordSession("demo", t0); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(v.Root, ".state")
	lock := filepath.Join(dir, "demo.lock")
	original, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := original.DACL()
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := original.Control()
	if err != nil {
		t.Fatal(err)
	}
	restoreFlags := windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	if control&windows.SE_DACL_PROTECTED != 0 {
		restoreFlags = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	}
	restore := func() error {
		return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.SECURITY_INFORMATION(restoreFlags), nil, nil, dacl, nil)
	}
	t.Cleanup(func() {
		if err := restore(); err != nil {
			t.Error("restore synthetic DACL:", err)
		}
	})
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	deny, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.FILE_WRITE_DATA, // FILE_ADD_FILE for a directory.
		AccessMode:        windows.DENY_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID,
			TrusteeType: windows.TRUSTEE_IS_USER, TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid)},
	}}, dacl)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, deny, nil); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if f != nil {
		f.Close()
		os.Remove(lock)
	}
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("fixture did not establish genuine ACL denial: %v", err)
	}
	changes, commits := 0, 0
	start := time.Now()
	err = v.updateState("demo", func(*projState) error { changes++; return nil }, func() error { commits++; return nil })
	elapsed := time.Since(start)
	var pe *os.PathError
	if !errors.As(err, &pe) || pe.Op != "open" || pe.Path != lock || !errors.Is(err, windows.ERROR_ACCESS_DENIED) || !os.IsPermission(err) {
		t.Fatalf("permanent ACL error was hidden or reclassified as contention: %v", err)
	}
	if elapsed < 500*time.Millisecond || elapsed > 2*time.Second {
		t.Fatalf("500ms acquisition budget changed or became unbounded: %v", elapsed)
	}
	if changes != 0 || commits != 0 {
		t.Fatalf("callback without acquired ownership: change=%d commit=%d", changes, commits)
	}
	assertKnownInvocations(t, v, 1)
	if _, err := os.Lstat(lock); !os.IsNotExist(err) {
		t.Fatalf("denied acquisition created a lock: %v", err)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if err := v.RecordSession("demo", t0); err != nil {
		t.Fatal("accounting did not recover after explicit ACL restoration:", err)
	}
	assertKnownInvocations(t, v, 2)
}

func TestSessionAccountingWindowsRetryCodesAreNarrow(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"access-denied", windows.ERROR_ACCESS_DENIED, true},
		{"sharing-violation", windows.ERROR_SHARING_VIOLATION, true},
		{"permission-sentinel", os.ErrPermission, false},
		{"exists", windows.ERROR_FILE_EXISTS, false},
		{"not-found", windows.ERROR_FILE_NOT_FOUND, false},
		{"io-error", windows.ERROR_IO_DEVICE, false},
		{"access-denied-text-only", errors.New("Access is denied."), false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryStateLockCreate(tc.err); got != tc.want {
				t.Fatalf("retry=%t want=%t error=%v", got, tc.want, tc.err)
			}
			wrapped := &os.PathError{Op: "open", Path: "synthetic.lock", Err: tc.err}
			if got := retryStateLockCreate(wrapped); got != tc.want {
				t.Fatalf("wrapped retry=%t want=%t error=%#v", got, tc.want, wrapped)
			}
		})
	}
}
