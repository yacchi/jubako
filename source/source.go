// Package source provides interfaces and implementations for configuration sources.
// A source represents where configuration data comes from and optionally where it can be saved.
// Sources are responsible only for I/O operations; parsing is handled by document.Document.
package source

import (
	"context"
	"errors"
)

// ErrSaveNotSupported is returned when Save is called on a source that doesn't support saving.
var ErrSaveNotSupported = errors.New("save not supported for this source")

// ErrSourceModified is returned when optimistic locking detects that the source
// has been modified since the last Load. This prevents overwriting external changes.
var ErrSourceModified = errors.New("source has been modified since last load")

// UpdateFunc is a function that generates new data to save.
// It receives the current bytes from the source (captured at a safe point)
// and returns the new bytes to write.
//
// The current bytes are provided so that formats supporting comment preservation
// can apply patches to the original data rather than regenerating from scratch.
type UpdateFunc func(current []byte) ([]byte, error)

// Source loads and optionally saves raw configuration data.
// The Source interface is implemented by various source types (files, byte slices, env vars, etc.).
// Sources are format-agnostic; they only handle raw bytes.
type Source interface {
	// Load reads the raw configuration data from the source.
	// The context can be used for cancellation and timeouts.
	Load(ctx context.Context) ([]byte, error)

	// Save writes data back to the source using optimistic locking.
	//
	// The updateFunc receives the current bytes captured at a safe checkpoint.
	// After updateFunc returns, the source verifies that the underlying data
	// hasn't changed since the checkpoint. If it has changed, ErrSourceModified
	// is returned and no write occurs.
	//
	// This pattern enables:
	//   - Optimistic concurrency control (detect external modifications)
	//   - Comment preservation (updateFunc can apply patches to current bytes)
	//   - Atomic updates (implementations can use temp file + rename)
	//
	// Returns ErrSaveNotSupported if the source doesn't support saving.
	// Returns ErrSourceModified if the source was modified externally.
	// The context can be used for cancellation and timeouts.
	//
	// Example:
	//   err := source.Save(ctx, func(current []byte) ([]byte, error) {
	//     return document.Apply(current, changeset)
	//   })
	Save(ctx context.Context, updateFunc UpdateFunc) error

	// CanSave returns true if the source supports saving.
	CanSave() bool
}
