package jktest

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/yacchi/jubako/document"
)

// testT is the minimal testing interface used by jktest utilities.
type testT interface {
	Helper()
	Fatalf(format string, args ...any)
	Errorf(format string, args ...any)
	Skip(args ...any)
	Skipf(format string, args ...any)
}

// require fails the test immediately if the condition is false.
func require(t testT, cond bool, format string, args ...any) {
	t.Helper()
	if !cond {
		t.Fatalf(format, args...)
	}
}

// requireNoError fails the test immediately if err is not nil.
func requireNoError(t testT, err error, format string, args ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf(format, args...)
	}
}

// check reports an error if the condition is false, but continues the test.
func check(t testT, cond bool, format string, args ...any) {
	t.Helper()
	if !cond {
		t.Errorf(format, args...)
	}
}

// isUnsupportedError checks if the error is an UnsupportedStructureError.
func isUnsupportedError(err error) bool {
	var unsupported *document.UnsupportedStructureError
	return errors.As(err, &unsupported)
}

// valuesEqual compares two values for equality, handling numeric type conversions.
func valuesEqual(got, want any) bool {
	// Handle nil
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}

	// Handle numeric type conversions (JSON/YAML may decode as different types)
	gotNum, gotIsNum := toFloat64(got)
	wantNum, wantIsNum := toFloat64(want)
	if gotIsNum && wantIsNum {
		return gotNum == wantNum
	}

	// Handle string comparison for env layer (which stores everything as strings)
	if gotStr, ok := got.(string); ok {
		if wantStr, ok := want.(string); ok {
			return gotStr == wantStr
		}
		// Compare string representation
		return gotStr == fmt.Sprintf("%v", want)
	}

	// Use reflect.DeepEqual for other types
	return reflect.DeepEqual(got, want)
}

// toFloat64 converts numeric types to float64 for comparison.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}
