// Package source provides interfaces and implementations for configuration sources.
// A source represents where configuration data comes from and optionally where it can be saved.
// Sources are responsible only for I/O operations; parsing is handled by document.Document.
package source

import (
	"context"
	"errors"

	"github.com/yacchi/jubako/types"
	"github.com/yacchi/jubako/watcher"
)

// ErrSaveNotSupported is returned when Save is called on a source that doesn't support saving.
var ErrSaveNotSupported = errors.New("save not supported for this source")

// ErrSourceModified is returned when optimistic locking detects that the source
// has been modified since the last Load. This prevents overwriting external changes.
var ErrSourceModified = errors.New("source has been modified since last load")

// SourceType is an alias for types.SourceType.
// This allows source implementations to use source.SourceType directly.
type SourceType = types.SourceType

// Standard source types.
const (
	TypeFS    SourceType = "fs"
	TypeBytes SourceType = "bytes"
)

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
	// Type returns the source type identifier (e.g., TypeFS, TypeBytes).
	// This is used for introspection and debugging.
	Type() SourceType

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

// WatchableSource is an optional interface that sources can implement
// to support change detection and hot reload.
//
// Sources can return different watcher types:
//   - SubscriptionWatcher: for event-based sources (fsnotify, etc.)
//   - PollingWatcher: for sources that need polling (remote APIs, etc.)
//   - NoopWatcher: for immutable sources (bytes.Source)
//
// If a source doesn't implement WatchableSource, layers can fall back
// to polling using the source's Load method.
type WatchableSource interface {
	// Watch returns a WatcherInitializer for this source.
	// The initializer is a factory function that creates a Watcher when called
	// with a fetch function and an optional operation mutex.
	//
	// This design separates the "what kind of watcher" decision (made by the source)
	// from the "how to synchronize" decision (made by the caller/layer).
	// The mutex is used by NewPolling and NewSubscription to wrap poll/fetch
	// operations, ensuring mutual exclusion with Load/Save operations.
	//
	// Example implementation (subscription-based):
	//
	//	func (s *Source) Watch() (watcher.WatcherInitializer, error) {
	//	    return watcher.NewSubscription(s), nil  // s implements SubscriptionHandler
	//	}
	//
	// Example implementation (polling-based):
	//
	//	func (s *Source) Watch() (watcher.WatcherInitializer, error) {
	//	    var lastETag *string
	//	    poll := func(ctx context.Context) (bool, []byte, error) {
	//	        // Poll logic using lastETag...
	//	    }
	//	    return watcher.NewPolling(poll), nil
	//	}
	Watch() (watcher.WatcherInitializer, error)
}
