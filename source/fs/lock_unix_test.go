//go:build unix

package fs

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestFileLockAndSupportDetection_Unix(t *testing.T) {
	t.Run("normal file locks", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "lock-*")
		if err != nil {
			t.Fatalf("CreateTemp() error = %v", err)
		}
		defer f.Close()

		unlock, err := fileLock(int(f.Fd()))
		if err != nil {
			t.Fatalf("fileLock() error = %v", err)
		}
		unlock()
	})

	t.Run("invalid fd returns error", func(t *testing.T) {
		if _, err := fileLock(-1); err == nil {
			t.Fatal("fileLock(-1) expected error, got nil")
		}
	})

	t.Run("isLockNotSupportedError recognizes sentinel errors", func(t *testing.T) {
		if !isLockNotSupportedError(syscall.ENOTSUP) {
			t.Fatal("ENOTSUP not recognized")
		}
		if !isLockNotSupportedError(syscall.EOPNOTSUPP) {
			t.Fatal("EOPNOTSUPP not recognized")
		}
		if !isLockNotSupportedError(syscall.ENOLCK) {
			t.Fatal("ENOLCK not recognized")
		}
		if isLockNotSupportedError(syscall.EBADF) {
			t.Fatal("EBADF incorrectly recognized")
		}
	})

	t.Run("unsupported locking falls back without error (best effort)", func(t *testing.T) {
		// Many Unix kernels return EOPNOTSUPP for non-file descriptors (e.g. pipes).
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("Pipe() error = %v", err)
		}
		defer r.Close()
		defer w.Close()

		unlock, lockErr := fileLock(int(r.Fd()))
		if lockErr != nil {
			// If this platform returns a different error, we can't force the
			// "not supported" branch without specialized filesystems.
			if isLockNotSupportedError(lockErr) {
				t.Fatalf("fileLock returned a lock-not-supported error: %v", lockErr)
			}
			if errors.Is(lockErr, syscall.EBADF) {
				t.Fatalf("fileLock returned EBADF: %v", lockErr)
			}
			t.Skipf("fileLock on pipe did not report lock-not-supported: %v", lockErr)
		}
		unlock()
	})
}
