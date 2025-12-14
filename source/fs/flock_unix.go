//go:build unix

// Package fs provides platform-specific file locking for Unix systems.
package fs

import (
	"errors"
	"syscall"
)

// flockExclusive acquires an exclusive lock on the file descriptor.
func flockExclusive(fd int) error {
	return syscall.Flock(fd, syscall.LOCK_EX)
}

// flockUnlock releases the lock on the file descriptor.
func flockUnlock(fd int) error {
	return syscall.Flock(fd, syscall.LOCK_UN)
}

// isLockNotSupportedError returns true if the error indicates that
// file locking is not supported by the filesystem.
func isLockNotSupportedError(err error) bool {
	// ENOTSUP (or EOPNOTSUPP) - operation not supported
	// ENOLCK - no locks available
	// These typically occur on network filesystems like NFS, SMB, etc.
	return errors.Is(err, syscall.ENOTSUP) ||
		errors.Is(err, syscall.EOPNOTSUPP) ||
		errors.Is(err, syscall.ENOLCK)
}
