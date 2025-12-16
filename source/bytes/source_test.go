package bytes

import (
	"context"
	"testing"
)

// TestSource_Load_ReturnsDefensiveCopy verifies that Load returns a copy of the data,
// not a reference to the internal slice. This is a bytes.Source-specific behavior.
func TestSource_Load_ReturnsDefensiveCopy(t *testing.T) {
	original := []byte("original data")
	src := New(original)

	// Load data
	loaded, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Modify the returned slice
	loaded[0] = 'X'

	// Load again and verify original data is unchanged
	loaded2, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if string(loaded2) != "original data" {
		t.Errorf("Source data was modified: got %q, want %q", string(loaded2), "original data")
	}
}

// TestFromString verifies the FromString convenience function.
func TestFromString(t *testing.T) {
	src := FromString("test data")

	loaded, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if string(loaded) != "test data" {
		t.Errorf("Load() = %q, want %q", string(loaded), "test data")
	}
}

// TestSource_Load_CancelledContext verifies context cancellation is respected.
func TestSource_Load_CancelledContext(t *testing.T) {
	src := New([]byte("data"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := src.Load(ctx)
	if err == nil {
		t.Error("Load() with cancelled context should return an error")
	}
}

// Note: Basic Load, CanSave, Save, Type, and Watch tests are covered by
// jktest.SourceTester compliance tests in jktest/source_test.go.
