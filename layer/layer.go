// Package layer provides interfaces and implementations for configuration layers.
// A layer represents a single configuration source with a priority.
// Layers are merged in order of priority to produce the final configuration.
package layer

import (
	"context"
	"sync"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/types"
	"github.com/yacchi/jubako/watcher"
)

// Priority defines the priority of a configuration layer.
// Higher values take precedence during merging.
type Priority int

// Name is a unique identifier for a configuration layer.
type Name string

// Layer represents a configuration source that can be loaded and optionally saved.
// Implementations handle the details of loading from various sources (files, environment, etc.).
// Priority is not part of the Layer interface - it's managed by the Store when adding layers.
//
// All Layer implementations must also implement types.DetailsFiller to provide
// metadata about the layer (source type, document format, watcher type, etc.).
type Layer interface {
	types.DetailsFiller

	// Name returns the unique identifier for this layer.
	Name() Name

	// Load reads configuration from the source and returns data as map[string]any.
	// The context can be used for cancellation and timeouts.
	Load(ctx context.Context) (map[string]any, error)

	// Save persists data back to the source using optimistic locking.
	// The changeset contains modifications since last load (for comment-preserving formats).
	// Returns source.ErrSaveNotSupported if the layer doesn't support saving.
	// Returns source.ErrSourceModified if the source was modified externally.
	Save(ctx context.Context, changeset document.JSONPatchSet) error

	// CanSave returns true if this layer supports saving.
	CanSave() bool

	// Watch returns a LayerWatcher for this layer.
	// The watcher should not be started yet; the caller will call Start.
	//
	// Use WithBaseConfig to pass the base watch configuration from the Store.
	// Use WithLayerWatchConfig to override specific options at the layer level.
	//
	// Layers that don't support watching should return a noop watcher
	// using NewNoopLayerWatcher().
	Watch(opts ...WatchOption) (LayerWatcher, error)
}

// basicLayer is the standard Layer implementation that combines a Source and Document.
// It is not exported; use the New() function to create instances.
//
// basicLayer provides operation-level synchronization via opMu, ensuring that
// Load, Save, and poll operations are mutually exclusive. This allows Source
// implementations to be simple I/O operations without their own synchronization.
type basicLayer struct {
	name   Name
	source source.Source
	doc    document.Document

	opMu sync.Mutex // protects Load/Save/poll from concurrent execution
}

// DocumentProvider is an optional interface that layers can implement
// to expose their underlying document. This is useful for accessing
// document-specific methods.
type DocumentProvider interface {
	Document() document.Document
}

// Ensure basicLayer implements Layer interface (which includes types.DetailsFiller).
var _ Layer = (*basicLayer)(nil)

// Ensure basicLayer implements DocumentProvider interface.
var _ DocumentProvider = (*basicLayer)(nil)

// New creates a new Layer with the given Source and Document.
// The Document is stateless and handles format parsing/serialization.
//
// The returned Layer also implements:
//   - DocumentProvider: for accessing the underlying document
//
// Example:
//
//	src := fs.New("~/.config/app.yaml")
//	layer := layer.New("user", src, yaml.New())
func New(name Name, src source.Source, doc document.Document) Layer {
	return &basicLayer{
		name:   name,
		source: src,
		doc:    doc,
	}
}

// Name returns the layer's name.
func (l *basicLayer) Name() Name {
	return l.name
}

// loadRawNoLock reads raw bytes from the source without mutex protection.
// This MUST be called with opMu held or from within a mutex-wrapped function.
// Used by watchers where the mutex is applied at a higher level via WatcherInitializer.
func (l *basicLayer) loadRawNoLock(ctx context.Context) ([]byte, error) {
	return l.source.Load(ctx)
}

// Load reads from the source via Document.Get and returns data as map[string]any.
// Load is synchronized with Save and poll operations via opMu.
func (l *basicLayer) Load(ctx context.Context) (map[string]any, error) {
	l.opMu.Lock()
	defer l.opMu.Unlock()

	data, err := l.loadRawNoLock(ctx)
	if err != nil {
		return nil, err
	}
	return l.doc.Get(data)
}

// Save generates output via Document.Apply and saves to the source.
// Uses optimistic locking to detect external modifications.
// Save is synchronized with Load and poll operations via opMu.
// Returns source.ErrSaveNotSupported if the source doesn't support saving.
// Returns source.ErrSourceModified if the source was modified externally.
func (l *basicLayer) Save(ctx context.Context, changeset document.JSONPatchSet) error {
	l.opMu.Lock()
	defer l.opMu.Unlock()

	return l.source.Save(ctx, func(current []byte) ([]byte, error) {
		return l.doc.Apply(current, changeset)
	})
}

// FillDetails populates the Details struct with metadata from this layer.
// It sets the source type, document format, watcher type, and delegates to the
// underlying source if it implements types.DetailsFiller for additional details.
func (l *basicLayer) FillDetails(d *types.Details) {
	// Set source type from source
	d.Source = l.source.Type()

	// Set format from document
	d.Format = l.doc.Format()

	// Set watcher type
	if ws, ok := l.source.(source.WatchableSource); ok {
		// Source provides its own watcher
		init, err := ws.Watch()
		if err == nil {
			// Create params for watcher initialization (only to get the type)
			w, err := init(watcher.WatcherInitializerParams{
				Fetch: func(ctx context.Context) (bool, []byte, error) {
					return true, nil, nil
				},
				OpMu: &l.opMu,
			})
			if err == nil {
				d.Watcher = w.Type()
			}
		}
	} else {
		// Fallback to polling
		d.Watcher = watcher.TypePolling
	}

	// Delegate to source if it implements DetailsFiller
	if df, ok := l.source.(types.DetailsFiller); ok {
		df.FillDetails(d)
	}
}

// Document returns the underlying document.
func (l *basicLayer) Document() document.Document {
	return l.doc
}

// CanSave returns true if this layer supports saving.
// The source must support saving for this to return true.
func (l *basicLayer) CanSave() bool {
	return l.source.CanSave()
}

// Watch implements the Layer interface.
// Returns a LayerWatcher that transforms source data into map[string]any.
//
// If the underlying source implements WatchableSource, its watcher is used.
// Otherwise, a fallback polling watcher is created using the source's Load method.
//
// # Configuration Flow
//
// Watch options are applied in two stages:
//  1. Store passes base config via WithBaseConfig (from StoreWatchConfig.WatcherOpts)
//  2. Layer-level overrides via WithLayerWatchConfig (for per-layer customization)
//
// The final config is computed by watchOptions.resolveConfig() which merges
// these two stages in order. This ensures Store-level defaults are respected
// while allowing layers to override specific settings.
//
// # Synchronization
//
// All I/O operations (poll/fetch) are synchronized with Load and Save operations
// via the layer's mutex (opMu), ensuring mutual exclusion. The mutex is passed to
// the WatcherInitializer, which wraps poll/fetch functions as appropriate.
func (l *basicLayer) Watch(opts ...WatchOption) (LayerWatcher, error) {
	// Resolve final config: base (from Store) + layer overrides
	cfg := ResolveWatchConfig(opts...)

	var init watcher.WatcherInitializer

	// Create a FetchFunc that wraps loadRawNoLock.
	// This always returns changed=true to indicate data was successfully fetched.
	// Actual change detection is performed by pollingWatcher using CompareFunc
	// to compare the fetched data with previously stored data.
	fetch := func(ctx context.Context) (bool, []byte, error) {
		data, err := l.loadRawNoLock(ctx)
		if err != nil {
			return false, nil, err
		}
		return true, data, nil
	}

	// Try to use source's native watcher if available
	if ws, ok := l.source.(source.WatchableSource); ok {
		var err error
		init, err = ws.Watch()
		if err != nil {
			return nil, err
		}
	} else {
		// Fallback to polling using the fetch function.
		// The mutex will be applied by NewPolling.
		init = watcher.NewPolling(fetch)
	}

	// Pass fetch, opMu, and resolved config to the initializer.
	// The initializer will wrap poll/fetch functions with mutex protection
	// and apply the configuration.
	w, err := init(watcher.WatcherInitializerParams{
		Fetch:  fetch,
		OpMu:   &l.opMu,
		Config: cfg,
	})
	if err != nil {
		return nil, err
	}
	return newLayerWatcher(w, l.doc), nil
}
