package fs

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

type fakeFileInfo struct {
	size int64
}

func (fi fakeFileInfo) Name() string       { return "fake" }
func (fi fakeFileInfo) Size() int64        { return fi.size }
func (fi fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fi fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fi fakeFileInfo) IsDir() bool        { return false }
func (fi fakeFileInfo) Sys() any           { return nil }

type fakeLockFile struct {
	statErr  error
	infoSize int64
	readErr  error
}

func (f *fakeLockFile) Stat() (os.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	return fakeFileInfo{size: f.infoSize}, nil
}

func (f *fakeLockFile) ReadAt(_ []byte, _ int64) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	return 0, nil
}

func (f *fakeLockFile) Close() error { return nil }
func (f *fakeLockFile) Fd() uintptr  { return 0 }

type fakeTempFile struct {
	path     string
	writeErr error
	syncErr  error
	closeErr error
}

func (f *fakeTempFile) Write(_ []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return 0, nil
}

func (f *fakeTempFile) Sync() error {
	return f.syncErr
}

func (f *fakeTempFile) Close() error {
	return f.closeErr
}

func (f *fakeTempFile) Name() string {
	return f.path
}

func withFSStubs(t *testing.T, fn func()) {
	t.Helper()

	origUserHomeDir := userHomeDir
	origReadFile := osReadFile
	origStat := osStat
	origMkdirAll := osMkdirAll
	origChmod := osChmod
	origRename := osRename
	origRemove := osRemove
	origFileLock := fileLockFunc
	origOpenFile := openFile
	origCreateTemp := createTemp

	t.Cleanup(func() {
		userHomeDir = origUserHomeDir
		osReadFile = origReadFile
		osStat = origStat
		osMkdirAll = origMkdirAll
		osChmod = origChmod
		osRename = origRename
		osRemove = origRemove
		fileLockFunc = origFileLock
		openFile = origOpenFile
		createTemp = origCreateTemp
	})

	fn()
}

func TestLoad_CanceledContext(t *testing.T) {
	s := New("config.json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Load(ctx); err == nil {
		t.Fatal("Load(canceled) expected error, got nil")
	}
}

func TestLoad_ResolvePathError(t *testing.T) {
	withFSStubs(t, func() {
		userHomeDir = func() (string, error) { return "", errors.New("home error") }

		s := New("~/.config/app.json")
		if _, err := s.Load(context.Background()); err == nil {
			t.Fatal("Load() expected error, got nil")
		}
	})
}

func TestResolvedPath_ExpandTildeError_ReturnsOriginal(t *testing.T) {
	withFSStubs(t, func() {
		userHomeDir = func() (string, error) { return "", errors.New("home error") }
		s := New("~/.config/app.json")
		if got := s.ResolvedPath(); got != "~/.config/app.json" {
			t.Fatalf("ResolvedPath() = %q, want %q", got, "~/.config/app.json")
		}
	})
}

func TestResolvePath_SkipsPathsThatFailExpansion(t *testing.T) {
	withFSStubs(t, func() {
		userHomeDir = func() (string, error) { return "", errors.New("home error") }

		dir := t.TempDir()
		existing := dir + "/config.json"
		if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		s := New("~/.config/missing.json", WithSearchPaths(existing))
		expanded, original, err := s.resolvePath()
		if err != nil {
			t.Fatalf("resolvePath() error = %v", err)
		}
		if expanded != existing || original != existing {
			t.Fatalf("resolvePath() = (%q, %q), want (%q, %q)", expanded, original, existing, existing)
		}
	})
}

func TestResolvePath_PrimaryExpandError_ReturnsError(t *testing.T) {
	withFSStubs(t, func() {
		userHomeDir = func() (string, error) { return "", errors.New("home error") }
		s := New("~/.config/missing.json")
		if _, _, err := s.resolvePath(); err == nil {
			t.Fatal("resolvePath() expected error, got nil")
		}
	})
}

func TestSave_ResolvePathError_ReturnsError(t *testing.T) {
	withFSStubs(t, func() {
		userHomeDir = func() (string, error) { return "", errors.New("home error") }
		s := New("~/.config/app.json")
		if err := s.Save(context.Background(), func(_ []byte) ([]byte, error) { return []byte("x"), nil }); err == nil {
			t.Fatal("Save() expected error, got nil")
		}
	})
}

func TestSave_LockError_ReturnsError(t *testing.T) {
	withFSStubs(t, func() {
		fileLockFunc = func(int) (func(), error) { return func() {}, errors.New("lock error") }
		openFile = func(string, int, os.FileMode) (lockFile, error) { return &fakeLockFile{}, nil }
		createTemp = func(string, string) (tempFile, error) { return &fakeTempFile{path: "tmp"}, nil }

		s := New("config.json")
		s.resolvedPath = "config.json"
		if err := s.Save(context.Background(), func(_ []byte) ([]byte, error) { return []byte("x"), nil }); err == nil {
			t.Fatal("Save() expected error, got nil")
		}
	})
}

func TestSave_StatError_ReturnsError(t *testing.T) {
	withFSStubs(t, func() {
		openFile = func(string, int, os.FileMode) (lockFile, error) {
			return &fakeLockFile{statErr: errors.New("stat error")}, nil
		}
		fileLockFunc = func(int) (func(), error) { return func() {}, nil }

		s := New("config.json")
		s.resolvedPath = "config.json"
		if err := s.Save(context.Background(), func(_ []byte) ([]byte, error) { return []byte("x"), nil }); err == nil {
			t.Fatal("Save() expected error, got nil")
		}
	})
}

func TestSave_ReadAtError_ReturnsError(t *testing.T) {
	withFSStubs(t, func() {
		openFile = func(string, int, os.FileMode) (lockFile, error) {
			return &fakeLockFile{infoSize: 1, readErr: errors.New("read error")}, nil
		}
		fileLockFunc = func(int) (func(), error) { return func() {}, nil }

		s := New("config.json")
		s.resolvedPath = "config.json"
		if err := s.Save(context.Background(), func(_ []byte) ([]byte, error) { return []byte("x"), nil }); err == nil {
			t.Fatal("Save() expected error, got nil")
		}
	})
}

func TestSave_TempFileWriteSyncCloseChmodRename_Errors(t *testing.T) {
	tests := []struct {
		name      string
		tmp       *fakeTempFile
		chmodErr  error
		renameErr error
	}{
		{name: "write", tmp: &fakeTempFile{path: "tmp", writeErr: errors.New("write error")}},
		{name: "sync", tmp: &fakeTempFile{path: "tmp", syncErr: errors.New("sync error")}},
		{name: "close", tmp: &fakeTempFile{path: "tmp", closeErr: errors.New("close error")}},
		{name: "chmod", tmp: &fakeTempFile{path: "tmp"}, chmodErr: errors.New("chmod error")},
		{name: "rename", tmp: &fakeTempFile{path: "tmp"}, renameErr: errors.New("rename error")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFSStubs(t, func() {
				openFile = func(string, int, os.FileMode) (lockFile, error) { return &fakeLockFile{}, nil }
				fileLockFunc = func(int) (func(), error) { return func() {}, nil }
				createTemp = func(string, string) (tempFile, error) { return tt.tmp, nil }
				osChmod = func(string, os.FileMode) error { return tt.chmodErr }
				osRename = func(string, string) error { return tt.renameErr }

				removed := false
				osRemove = func(string) error {
					removed = true
					return nil
				}

				s := New("config.json")
				s.resolvedPath = "config.json"
				if err := s.Save(context.Background(), func(_ []byte) ([]byte, error) { return []byte("x"), nil }); err == nil {
					t.Fatal("Save() expected error, got nil")
				}
				if !removed {
					t.Fatal("expected temporary file cleanup")
				}
			})
		})
	}
}
