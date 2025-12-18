package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/types"
	"github.com/yacchi/jubako/watcher"
)

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}

	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"config.yaml", "config.yaml"},
		{"~", home},
		{"~/config.yaml", filepath.Join(home, "config.yaml")},
		{"~someone/config.yaml", "~someone/config.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := expandTilde(tt.in)
			if err != nil {
				t.Fatalf("expandTilde() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("expandTilde(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolvePathAndResolvedPath(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "primary.json")
	alt := filepath.Join(dir, "alt.json")

	if err := os.WriteFile(alt, []byte("alt"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s := New(primary, WithSearchPaths(alt))
	details := &types.Details{}
	s.FillDetails(details)
	if details.Path != primary {
		t.Fatalf("FillDetails().Path = %q, want %q", details.Path, primary)
	}
	if !s.CanSave() {
		t.Fatal("CanSave() = false, want true")
	}

	got := s.ResolvedPath()
	if got != primary {
		t.Fatalf("ResolvedPath() before Load = %q, want %q", got, primary)
	}

	data, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(data) != "alt" {
		t.Fatalf("Load() data = %q, want %q", string(data), "alt")
	}
	if got := s.ResolvedPath(); got != alt {
		t.Fatalf("ResolvedPath() after Load = %q, want %q", got, alt)
	}
}

func TestLoad_NoFilesFound(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "missing.json")
	s := New(primary, WithSearchPaths(filepath.Join(dir, "also-missing.json")))

	_, err := s.Load(context.Background())
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	// Check that the error is source.ErrNotExist
	if !errors.Is(err, source.ErrNotExist) {
		t.Fatalf("error = %q, want source.ErrNotExist", err.Error())
	}
	// Also check that the underlying os.ErrNotExist is preserved
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %q, underlying os.ErrNotExist should be preserved", err.Error())
	}
}

func TestSave_WritesAtomicallyAndSetsMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nested", "config.json")

	s := New(target, WithFileMode(0o600), WithDirMode(0o700))

	// Cancellation is checked up front.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Save(canceled, func(_ []byte) ([]byte, error) { return nil, nil }); err == nil {
		t.Fatal("Save(canceled) expected error, got nil")
	}

	err := s.Save(context.Background(), func(_ []byte) ([]byte, error) {
		return []byte("content"), nil
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(b) != "content" {
		t.Fatalf("file content = %q, want %q", string(b), "content")
	}

	st, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := st.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode = %o, want %o", got, 0o600)
	}
}

func TestSave_UpdateFuncError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.json")

	s := New(target)
	wantErr := errors.New("update error")
	err := s.Save(context.Background(), func(_ []byte) ([]byte, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Save() error = %v, want %v", err, wantErr)
	}
}

func TestSave_MkdirAllFailure(t *testing.T) {
	dir := t.TempDir()

	// Create a file where a directory should be, so MkdirAll fails.
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	target := filepath.Join(blocker, "config.json")
	s := New(target)
	if err := s.Save(context.Background(), func(_ []byte) ([]byte, error) { return []byte("x"), nil }); err == nil {
		t.Fatal("Save() expected error, got nil")
	}
}

func TestSave_OpenFileFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "config.json")

	// Make the directory non-writable so OpenFile(O_CREATE) fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	defer os.Chmod(dir, 0o700)

	s := New(target)
	if err := s.Save(context.Background(), func(_ []byte) ([]byte, error) { return []byte("x"), nil }); err == nil {
		t.Fatal("Save() expected error, got nil")
	}
}

func TestSave_CreateTempFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "config.json")

	// Ensure the target exists so OpenFile succeeds even when dir becomes non-writable.
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	defer os.Chmod(dir, 0o700)

	s := New(target)
	if err := s.Save(context.Background(), func(b []byte) ([]byte, error) { return b, nil }); err == nil {
		t.Fatal("Save() expected error, got nil")
	}
}

// TestSource_Watch verifies fs.Source returns a valid subscription watcher.
// Basic watcher tests are covered by jktest.SourceTester compliance tests.
func TestSource_Watch(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.json")
	if err := os.WriteFile(target, []byte(`{"key": "initial"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s := New(target)
	init, err := s.Watch()
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if init == nil {
		t.Fatal("Watch() returned nil initializer")
	}

	// Create watcher with test params
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

	// Verify it's a subscription watcher (fs-specific)
	if w.Type() != "subscription" {
		t.Errorf("Watch().Type() = %q, want %q", w.Type(), "subscription")
	}
}

func TestSource_Subscribe(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.json")
	if err := os.WriteFile(target, []byte(`{"key": "initial"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s := New(target)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Track event-only notifications (data=nil, err=nil)
	var notified bool
	var mu sync.Mutex
	notify := func(data []byte, err error) {
		mu.Lock()
		// With the new design, Subscribe sends event-only notifications (nil, nil)
		// The subscriber is expected to fetch data separately
		if data == nil && err == nil {
			notified = true
		}
		mu.Unlock()
	}

	stop, err := s.Subscribe(ctx, notify)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer stop(context.Background())

	// Write to file to trigger notification
	time.Sleep(50 * time.Millisecond) // Wait for watcher to be ready
	if err := os.WriteFile(target, []byte(`{"key": "updated"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Wait for event-only notification
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := notified
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	if !notified {
		t.Error("expected event-only notification (nil, nil), got none")
	}
	mu.Unlock()

	// Verify that Load returns the updated data
	data, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(data) != `{"key": "updated"}` {
		t.Errorf("Load() = %q, want %q", string(data), `{"key": "updated"}`)
	}
}

func TestSource_Subscribe_DirectoryNotFound(t *testing.T) {
	s := New("/nonexistent/dir/config.json")

	ctx := context.Background()
	_, err := s.Subscribe(ctx, func(data []byte, err error) {})
	if err == nil {
		t.Error("Subscribe() expected error for nonexistent directory, got nil")
	}
}
