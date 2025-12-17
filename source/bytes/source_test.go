package bytes

import (
	"context"
	"sync"
	"testing"

	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
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

// TestSource_Type verifies the Type method.
func TestSource_Type(t *testing.T) {
	src := New([]byte("data"))
	if got := src.Type(); got != "bytes" {
		t.Errorf("Type() = %q, want %q", got, "bytes")
	}
}

// TestSource_CanSave verifies that bytes source does not support saving.
func TestSource_CanSave(t *testing.T) {
	src := New([]byte("data"))
	if src.CanSave() {
		t.Error("CanSave() = true, want false")
	}
}

// TestSource_Save verifies that Save returns ErrSaveNotSupported.
func TestSource_Save(t *testing.T) {
	src := New([]byte("data"))
	err := src.Save(context.Background(), func([]byte) ([]byte, error) {
		return nil, nil
	})
	if err != source.ErrSaveNotSupported {
		t.Errorf("Save() error = %v, want %v", err, source.ErrSaveNotSupported)
	}
}

// TestSource_Watch verifies the Watch method returns a NoopWatcher.
func TestSource_Watch(t *testing.T) {
	src := New([]byte("data"))

	init, err := src.Watch()
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if init == nil {
		t.Fatal("Watch() returned nil initializer")
	}

	var mu sync.Mutex
	w, err := init(watcher.WatcherInitializerParams{
		Fetch: func(ctx context.Context) (bool, []byte, error) {
			return true, nil, nil
		},
		OpMu: &mu,
	})
	if err != nil {
		t.Fatalf("WatcherInitializer() error = %v", err)
	}
	if w == nil {
		t.Fatal("WatcherInitializer() returned nil watcher")
	}

	// Verify it's a noop watcher
	if got := w.Type(); got != watcher.TypeNoop {
		t.Errorf("Watch().Type() = %v, want %v", got, watcher.TypeNoop)
	}
}
