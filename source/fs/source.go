// Package fs provides a file system based configuration source.
package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/types"
	"github.com/yacchi/jubako/watcher"
)

type lockFile interface {
	Stat() (os.FileInfo, error)
	ReadAt(p []byte, off int64) (n int, err error)
	Close() error
	Fd() uintptr
}

type tempFile interface {
	Write(p []byte) (n int, err error)
	Sync() error
	Close() error
	Name() string
}

var (
	userHomeDir  = os.UserHomeDir
	osReadFile   = os.ReadFile
	osStat       = os.Stat
	osMkdirAll   = os.MkdirAll
	osChmod      = os.Chmod
	osRename     = os.Rename
	osRemove     = os.Remove
	fileLockFunc = fileLock

	openFile = func(name string, flag int, perm os.FileMode) (lockFile, error) {
		return os.OpenFile(name, flag, perm)
	}
	createTemp = func(dir, pattern string) (tempFile, error) {
		return os.CreateTemp(dir, pattern)
	}
)

// fileLock attempts to acquire an exclusive lock on the given file descriptor.
// Returns a function to release the lock. If locking is not supported by the
// filesystem, both the error and unlock function will be nil - the operation
// proceeds without locking since there's no safe alternative anyway.
// The unlock function is safe to call even if it's nil.
func fileLock(fd int) (unlock func(), err error) {
	if err := flockExclusive(fd); err != nil {
		// Check if locking is not supported by this filesystem
		if isLockNotSupportedError(err) {
			// Proceed without locking - no safe alternative exists
			return func() {}, nil
		}
		return nil, err
	}
	return func() { flockUnlock(fd) }, nil
}

// Default permission modes.
const (
	DefaultFileMode = 0644
	DefaultDirMode  = 0755
)

// Source loads and saves raw configuration data from/to a file.
type Source struct {
	path         string
	searchPaths  []string
	resolvedPath string // cached path after resolution
	fileMode     os.FileMode
	dirMode      os.FileMode
}

// Ensure Source implements the source.Source interface.
var _ source.Source = (*Source)(nil)

// Ensure Source implements the source.WatchableSource interface.
var _ source.WatchableSource = (*Source)(nil)

// Option configures a Source.
type Option func(*Source)

// WithFileMode sets the file permission mode used when saving.
// Default is 0644.
func WithFileMode(mode os.FileMode) Option {
	return func(s *Source) {
		s.fileMode = mode
	}
}

// WithDirMode sets the directory permission mode used when creating parent directories.
// Default is 0755.
func WithDirMode(mode os.FileMode) Option {
	return func(s *Source) {
		s.dirMode = mode
	}
}

// WithSearchPaths adds additional paths to search for the configuration file.
// During Load, files are searched in order: primary path first, then search paths.
// The first existing file is used. If no file exists, the primary path is used.
// During Save, the resolved path (found file or primary path) is used.
func WithSearchPaths(paths ...string) Option {
	return func(s *Source) {
		s.searchPaths = append(s.searchPaths, paths...)
	}
}

// New creates a source that reads from and writes to a file.
// The path can be absolute or relative. Tilde (~) expansion is supported.
//
// Example:
//
//	src := fs.New("~/.config/app/config.yaml")
//	src := fs.New("/etc/app/config.yaml")
//	src := fs.New(".app.yaml")
//	src := fs.New("config.yaml", fs.WithFileMode(0600), fs.WithDirMode(0700))
//	src := fs.New("~/.config/app/config.yaml",
//	    fs.WithSearchPaths("/etc/app/config.yaml", ".app.yaml"))
func New(path string, opts ...Option) *Source {
	s := &Source{
		path:     path,
		fileMode: DefaultFileMode,
		dirMode:  DefaultDirMode,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Load implements the source.Source interface.
// If search paths are configured, files are searched in order:
// primary path first, then search paths. The first existing file is loaded.
// If no file exists, an error is returned for the primary path.
func (s *Source) Load(ctx context.Context) ([]byte, error) {
	// Check for cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	resolvedPath, originalPath, err := s.resolvePath()
	if err != nil {
		return nil, err
	}

	// Read file
	data, err := osReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %q: %w", originalPath, err)
	}

	// Cache the resolved path for subsequent operations
	s.resolvedPath = resolvedPath

	return data, nil
}

// Save implements the source.Source interface with file locking.
// The updateFunc receives current file contents and returns the new contents to write.
//
// File locking (flock) is used to prevent concurrent modifications.
// If the filesystem doesn't support locking, the operation proceeds without it
// since there's no safe alternative anyway.
//
// The write is performed atomically by writing to a temporary file first,
// then renaming it to the target path. Parent directories are created if
// they do not exist.
func (s *Source) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	// Check for cancellation
	if err := ctx.Err(); err != nil {
		return err
	}

	// Use cached resolved path if available, otherwise resolve
	targetPath := s.resolvedPath
	if targetPath == "" {
		var err error
		targetPath, _, err = s.resolvePath()
		if err != nil {
			return err
		}
	}

	// Ensure parent directory exists
	dir := filepath.Dir(targetPath)
	if err := osMkdirAll(dir, s.dirMode); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	// Open or create file for locking
	// We use O_RDWR|O_CREATE to handle both existing and new files
	lockFile, err := openFile(targetPath, os.O_RDWR|os.O_CREATE, s.fileMode)
	if err != nil {
		return fmt.Errorf("failed to open file %q for locking: %w", targetPath, err)
	}
	defer lockFile.Close()

	// Acquire exclusive lock
	// If locking is not supported, fileLock returns (func(){}, nil) and we proceed
	unlock, err := fileLockFunc(int(lockFile.Fd()))
	if err != nil {
		return fmt.Errorf("failed to acquire lock on %q: %w", targetPath, err)
	}
	defer unlock()

	// Read current file contents through the locked file handle
	var currentData []byte
	stat, err := lockFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file %q: %w", targetPath, err)
	}
	if stat.Size() > 0 {
		currentData = make([]byte, stat.Size())
		if _, err := lockFile.ReadAt(currentData, 0); err != nil {
			return fmt.Errorf("failed to read current file %q: %w", targetPath, err)
		}
	}

	// Call updateFunc with current data
	newData, err := updateFunc(currentData)
	if err != nil {
		return err
	}

	// Atomic write via temp file + rename
	tmpFile, err := createTemp(dir, ".jubako-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temporary file on error
	success := false
	defer func() {
		if !success {
			osRemove(tmpPath)
		}
	}()

	// Write data to temporary file
	if _, err := tmpFile.Write(newData); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}

	// Sync to ensure data is flushed to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Set permissions
	if err := osChmod(tmpPath, s.fileMode); err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	// Atomic rename (lock is still held, ensuring exclusive access)
	if err := osRename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("failed to rename temporary file to %q: %w", targetPath, err)
	}

	success = true
	return nil
}

// Type returns the source type identifier.
func (s *Source) Type() source.SourceType {
	return source.TypeFS
}

// FillDetails implements types.DetailsFiller.
// It populates the Details struct with the file path.
func (s *Source) FillDetails(d *types.Details) {
	d.Path = s.path
}

// ResolvedPath returns the actual file path being used after resolution.
// This may differ from Path() if a search path was used.
// Returns the primary path if no file has been loaded yet.
func (s *Source) ResolvedPath() string {
	if s.resolvedPath != "" {
		return s.resolvedPath
	}
	expanded, err := expandTilde(s.path)
	if err != nil {
		return s.path
	}
	return expanded
}

// resolvePath finds the first existing file from the search paths.
// Returns (expandedPath, originalPath, error).
// If no file exists, returns the expanded primary path.
func (s *Source) resolvePath() (expanded string, original string, err error) {
	// Build list of paths to search: primary path first, then search paths
	allPaths := make([]string, 0, 1+len(s.searchPaths))
	allPaths = append(allPaths, s.path)
	allPaths = append(allPaths, s.searchPaths...)

	// Search for the first existing file
	for _, p := range allPaths {
		expanded, err := expandTilde(p)
		if err != nil {
			continue
		}
		if _, statErr := osStat(expanded); statErr == nil {
			return expanded, p, nil
		}
	}

	// No file found, return primary path (will likely cause an error on read)
	expanded, err = expandTilde(s.path)
	if err != nil {
		return "", s.path, fmt.Errorf("failed to expand path %q: %w", s.path, err)
	}
	return expanded, s.path, nil
}

// CanSave returns true because file system sources support saving.
func (s *Source) CanSave() bool {
	return true
}

// expandTilde expands tilde (~) in the path.
// Handles both "~" (home directory) and "~/path" (path under home).
func expandTilde(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}

	homeDir, err := userHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to expand home directory: %w", err)
	}

	if len(path) == 1 {
		// Just "~"
		return homeDir, nil
	}

	if path[1] == '/' || path[1] == filepath.Separator {
		// "~/path" - join home with the path after "~/"
		return filepath.Join(homeDir, path[2:]), nil
	}

	// "~something" - not a valid home expansion, return as-is
	return path, nil
}

// Subscribe implements the watcher.SubscriptionHandler interface.
// It sets up fsnotify-based file watching and calls notify when the file changes.
//
// This uses the event-only notification pattern: notify(nil, nil) is called
// when a change is detected, and the subscriber fetches data separately.
func (s *Source) Subscribe(ctx context.Context, notify watcher.NotifyFunc) (watcher.StopFunc, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	path := s.ResolvedPath()

	// Watch the directory containing the file rather than the file itself.
	// This handles atomic writes (temp file + rename) and file recreation.
	dir := filepath.Dir(path)
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, fmt.Errorf("failed to watch directory %q: %w", dir, err)
	}

	filename := filepath.Base(path)

	go func() {
		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				// Only process events for our specific file
				if filepath.Base(event.Name) != filename {
					continue
				}
				// Handle write, create, and rename events
				// Notify with (nil, nil) to indicate event-only notification.
				// The subscriber will fetch data using the synchronized fetch function.
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
					notify(nil, nil)
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				notify(nil, err)
			case <-ctx.Done():
				return
			}
		}
	}()

	stop := func(ctx context.Context) error {
		return w.Close()
	}

	return stop, nil
}

// Watch implements the source.WatchableSource interface.
// Returns a WatcherInitializer that creates a SubscriptionWatcher using fsnotify.
func (s *Source) Watch() (watcher.WatcherInitializer, error) {
	return watcher.NewSubscription(watcher.SubscriptionHandlerFunc(s.Subscribe)), nil
}
