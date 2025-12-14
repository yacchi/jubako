//go:build windows

// Package fs provides platform-specific file locking for Windows.
package fs

// flockExclusive is a no-op on Windows.
// Windows file locking uses different mechanisms (LockFileEx).
// For now, we skip locking on Windows.
func flockExclusive(fd int) error {
	return nil
}

// flockUnlock is a no-op on Windows.
func flockUnlock(fd int) error {
	return nil
}

// isLockNotSupportedError always returns false on Windows
// since we don't attempt locking.
func isLockNotSupportedError(err error) bool {
	return false
}
