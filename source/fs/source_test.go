package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	if got := s.Path(); got != primary {
		t.Fatalf("Path() = %q, want %q", got, primary)
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
	if !strings.Contains(err.Error(), "failed to read file") {
		t.Fatalf("error = %q, want to contain %q", err.Error(), "failed to read file")
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
