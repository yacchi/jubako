package document

import (
	"errors"
	"testing"
)

func TestPathNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "simple path",
			path:     "/server/port",
			expected: "path not found: /server/port",
		},
		{
			name:     "nested path",
			path:     "/database/connections/primary",
			expected: "path not found: /database/connections/primary",
		},
		{
			name:     "array index path",
			path:     "/servers/0/name",
			expected: "path not found: /servers/0/name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &PathNotFoundError{Path: tt.path}
			if got := err.Error(); got != tt.expected {
				t.Errorf("PathNotFoundError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestInvalidPathError(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		reason   string
		expected string
	}{
		{
			name:     "missing leading slash",
			path:     "server/port",
			reason:   "must start with '/'",
			expected: `invalid path "server/port": must start with '/'`,
		},
		{
			name:     "invalid array index",
			path:     "/servers/abc",
			reason:   "array index must be a number",
			expected: `invalid path "/servers/abc": array index must be a number`,
		},
		{
			name:     "cannot set root",
			path:     "",
			reason:   "cannot set root document",
			expected: `invalid path "": cannot set root document`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &InvalidPathError{Path: tt.path, Reason: tt.reason}
			if got := err.Error(); got != tt.expected {
				t.Errorf("InvalidPathError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTypeMismatchError(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
		actual   string
		want     string
	}{
		{
			name:     "expected mapping got scalar",
			path:     "/server",
			expected: "mapping",
			actual:   "scalar",
			want:     `type mismatch at "/server": expected mapping, got scalar`,
		},
		{
			name:     "expected sequence got mapping",
			path:     "/servers",
			expected: "sequence",
			actual:   "mapping",
			want:     `type mismatch at "/servers": expected sequence, got mapping`,
		},
		{
			name:     "expected mapping or sequence got scalar",
			path:     "/config/nested",
			expected: "mapping or sequence",
			actual:   "scalar",
			want:     `type mismatch at "/config/nested": expected mapping or sequence, got scalar`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &TypeMismatchError{Path: tt.path, Expected: tt.expected, Actual: tt.actual}
			if got := err.Error(); got != tt.want {
				t.Errorf("TypeMismatchError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestErrorUnwrapping ensures error types can be properly detected.
func TestErrorUnwrapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		fn   func(error) bool
	}{
		{
			name: "PathNotFoundError is not InvalidPathError",
			err:  &PathNotFoundError{Path: "/test"},
			fn: func(err error) bool {
				var target *InvalidPathError
				return !errors.As(err, &target)
			},
		},
		{
			name: "InvalidPathError can be detected",
			err:  &InvalidPathError{Path: "/test", Reason: "test"},
			fn: func(err error) bool {
				var target *InvalidPathError
				return errors.As(err, &target)
			},
		},
		{
			name: "TypeMismatchError can be detected",
			err:  &TypeMismatchError{Path: "/test", Expected: "mapping", Actual: "scalar"},
			fn: func(err error) bool {
				var target *TypeMismatchError
				return errors.As(err, &target)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.fn(tt.err) {
				t.Errorf("error check failed for %T", tt.err)
			}
		})
	}
}
