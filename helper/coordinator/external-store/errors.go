package externalstore

import "errors"

// ErrNotExist is a sentinel error for external secret entries that do not exist.
// Use errors.Is(err, externalstore.ErrNotExist) to check for this condition.
var ErrNotExist = errors.New("external secret does not exist")

// NotExistError wraps an underlying error while also matching ErrNotExist.
// This allows callers to preserve backend-specific errors while exposing a
// shared "not found" contract at the helper boundary.
type NotExistError struct {
	Key string
	Err error
}

// NewNotExistError creates a NotExistError for the given external-store key.
func NewNotExistError(key string, err error) *NotExistError {
	return &NotExistError{Key: key, Err: err}
}

// Error returns the formatted error string.
func (e *NotExistError) Error() string {
	if e.Err != nil {
		return "external secret not found: " + e.Key + ": " + e.Err.Error()
	}
	return "external secret not found: " + e.Key
}

// Unwrap returns the underlying error.
func (e *NotExistError) Unwrap() error {
	return e.Err
}

// Is reports whether this error matches ErrNotExist.
func (e *NotExistError) Is(target error) bool {
	return target == ErrNotExist
}
