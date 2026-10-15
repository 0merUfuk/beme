//go:build !windows

package brain

func retryStateLockCreate(error) bool { return false }
