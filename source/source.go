// Package source provides interfaces and implementations for configuration sources.
// A source represents where configuration data comes from and optionally where it can be saved.
// Sources are responsible only for I/O operations; parsing is handled by document.Parser.
package source

import (
	"context"
	"errors"
)

// ErrSaveNotSupported is returned when Save is called on a source that doesn't support saving.
var ErrSaveNotSupported = errors.New("save not supported for this source")

// Source loads and optionally saves raw configuration data.
// The Source interface is implemented by various source types (files, byte slices, env vars, etc.).
// Sources are format-agnostic; they only handle raw bytes.
type Source interface {
	// Load reads the raw configuration data from the source.
	// The context can be used for cancellation and timeouts.
	Load(ctx context.Context) ([]byte, error)

	// Save writes raw data back to the source.
	// Returns ErrSaveNotSupported if the source doesn't support saving.
	// The context can be used for cancellation and timeouts.
	Save(ctx context.Context, data []byte) error

	// CanSave returns true if the source supports saving.
	CanSave() bool
}
