// Package bytes provides a byte slice based configuration source.
// This source is read-only; Save operations return ErrSaveNotSupported.
package bytes

import (
	"context"

	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
)

// Source loads raw configuration data from a byte slice.
// This source does not support saving.
type Source struct {
	data []byte
}

// Ensure Source implements the source.Source interface.
var _ source.Source = (*Source)(nil)

// Ensure Source implements the source.WatchableSource interface.
var _ source.WatchableSource = (*Source)(nil)

// New creates a source from raw bytes.
//
// Example:
//
//	data := []byte("server:\n  port: 8080")
//	src := bytes.New(data)
func New(data []byte) *Source {
	return &Source{
		data: data,
	}
}

// FromString creates a source from a string.
// This is a convenience function that converts the string to bytes.
//
// Example:
//
//	src := bytes.FromString("server:\n  port: 8080")
func FromString(data string) *Source {
	return New([]byte(data))
}

// Type returns the source type identifier.
func (s *Source) Type() source.SourceType {
	return source.TypeBytes
}

// Load implements the source.Source interface.
// Returns a copy of the data to prevent callers from modifying the source.
func (s *Source) Load(ctx context.Context) ([]byte, error) {
	// Check for cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Return a copy to prevent callers from modifying the source data
	result := make([]byte, len(s.data))
	copy(result, s.data)
	return result, nil
}

// Save implements the source.Source interface.
// This source does not support saving and always returns ErrSaveNotSupported.
func (s *Source) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	return source.ErrSaveNotSupported
}

// CanSave returns false because byte slice sources do not support saving.
func (s *Source) CanSave() bool {
	return false
}

// Watch implements the source.WatchableSource interface.
// Returns a WatcherInitializer that creates a NoopWatcher because byte slice
// sources are immutable and never change.
func (s *Source) Watch() (watcher.WatcherInitializer, error) {
	return watcher.NewNoop(), nil
}
