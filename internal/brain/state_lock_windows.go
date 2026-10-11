package brain

import (
	"errors"

	"golang.org/x/sys/windows"
)

// Windows can deny CREATE_NEW while the previous lock is delete-pending, or
// while another handle refuses sharing. ACCESS_DENIED can also mean a permanent
// ACL refusal: only retry acquisition within its existing deadline, retaining
// the original error if no exclusive create succeeds. Never infer ownership or
// permission to remove a node from these ambiguous failures.
func retryStateLockCreate(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
