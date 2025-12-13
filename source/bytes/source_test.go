package bytes

import (
	"context"
	"testing"
)

func TestSource_Load(t *testing.T) {
	data := []byte("server:\n  port: 8080")
	src := New(data)

	loaded, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if string(loaded) != string(data) {
		t.Errorf("Load() = %q, want %q", string(loaded), string(data))
	}
}

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

func TestSource_CanSave(t *testing.T) {
	src := New([]byte("data"))

	if src.CanSave() {
		t.Error("CanSave() = true, want false")
	}
}

func TestSource_Save_ReturnsError(t *testing.T) {
	src := New([]byte("data"))

	err := src.Save(context.Background(), []byte("new data"))
	if err == nil {
		t.Error("Save() should return an error")
	}
}

func TestSource_Load_CancelledContext(t *testing.T) {
	src := New([]byte("data"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := src.Load(ctx)
	if err == nil {
		t.Error("Load() with cancelled context should return an error")
	}
}
