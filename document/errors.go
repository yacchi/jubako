package document

import "fmt"

// PathNotFoundError is returned when a path does not exist in the document.
type PathNotFoundError struct {
	Path string
}

func (e *PathNotFoundError) Error() string {
	return fmt.Sprintf("path not found: %s", e.Path)
}

// InvalidPathError is returned when a path is malformed or invalid.
type InvalidPathError struct {
	Path   string
	Reason string
}

func (e *InvalidPathError) Error() string {
	return fmt.Sprintf("invalid path %q: %s", e.Path, e.Reason)
}

// TypeMismatchError is returned when an operation expects a different type.
type TypeMismatchError struct {
	Path     string
	Expected string
	Actual   string
}

func (e *TypeMismatchError) Error() string {
	return fmt.Sprintf("type mismatch at %q: expected %s, got %s", e.Path, e.Expected, e.Actual)
}

// UnsupportedStructureError is returned when MarshalTestData encounters
// a data structure that cannot be represented in the document format.
type UnsupportedStructureError struct {
	Path   string
	Reason string
}

func (e *UnsupportedStructureError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("unsupported structure: %s", e.Reason)
	}
	return fmt.Sprintf("unsupported structure at %q: %s", e.Path, e.Reason)
}

// Unsupported creates an UnsupportedStructureError with the given reason.
// Use this for simple cases without a specific path.
//
// Example:
//
//	return nil, document.Unsupported("arrays")
func Unsupported(reason string) *UnsupportedStructureError {
	return &UnsupportedStructureError{Reason: reason}
}

// UnsupportedAt creates an UnsupportedStructureError at a specific path.
//
// Example:
//
//	return nil, document.UnsupportedAt("/items/0", "arrays")
func UnsupportedAt(path, reason string) *UnsupportedStructureError {
	return &UnsupportedStructureError{Path: path, Reason: reason}
}
