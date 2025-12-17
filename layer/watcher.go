package layer

import (
	"context"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/watcher"
)

// LayerWatchResult represents the result of a layer watch cycle.
type LayerWatchResult struct {
	// Data is the latest data from the layer, parsed into map[string]any.
	Data map[string]any

	// Error is set if the watch encountered an error.
	Error error
}

// LayerWatcher watches a layer for changes and notifies via a channel.
type LayerWatcher interface {
	// Start begins watching for changes.
	// Configuration is provided at creation time via Layer.Watch().
	Start(ctx context.Context) error

	// Stop stops watching and releases resources.
	Stop(ctx context.Context) error

	// Results returns a channel that receives watch results.
	Results() <-chan LayerWatchResult
}

// layerWatcher wraps a source-level watcher and transforms []byte to map[string]any.
type layerWatcher struct {
	watcher watcher.Watcher
	doc     document.Document
	results chan LayerWatchResult
}

func newLayerWatcher(w watcher.Watcher, doc document.Document) *layerWatcher {
	return &layerWatcher{
		watcher: w,
		doc:     doc,
	}
}

// Start begins watching and transforms results from []byte to map[string]any.
// Configuration is provided at creation time via Layer.Watch().
func (w *layerWatcher) Start(ctx context.Context) error {
	w.results = make(chan LayerWatchResult)

	if err := w.watcher.Start(ctx); err != nil {
		close(w.results)
		return err
	}

	go func() {
		defer close(w.results)
		for result := range w.watcher.Results() {
			if result.Error != nil {
				w.results <- LayerWatchResult{Error: result.Error}
				continue
			}
			data, err := w.doc.Get(result.Data)
			w.results <- LayerWatchResult{Data: data, Error: err}
		}
	}()

	return nil
}

// Stop stops the underlying watcher.
func (w *layerWatcher) Stop(ctx context.Context) error {
	return w.watcher.Stop(ctx)
}

// Results returns the channel receiving layer watch results.
func (w *layerWatcher) Results() <-chan LayerWatchResult {
	return w.results
}

// noopLayerWatcher is a LayerWatcher that never reports changes.
// Use this for layers that don't support watching (e.g., env layer).
type noopLayerWatcher struct {
	results chan LayerWatchResult
	stopCh  chan struct{}
	running bool
}

// NewNoopLayerWatcher creates a LayerWatcher that never reports changes.
// This is useful for layers that don't support watching, such as environment
// variable layers that are read once at startup.
func NewNoopLayerWatcher() LayerWatcher {
	return &noopLayerWatcher{}
}

// Start begins the noop watcher. It will block until Stop is called but never emit results.
func (w *noopLayerWatcher) Start(ctx context.Context) error {
	if w.running {
		return nil
	}
	w.running = true
	w.results = make(chan LayerWatchResult)
	w.stopCh = make(chan struct{})

	go func() {
		defer close(w.results)
		select {
		case <-ctx.Done():
		case <-w.stopCh:
		}
	}()

	return nil
}

// Stop stops the noop watcher.
func (w *noopLayerWatcher) Stop(ctx context.Context) error {
	if !w.running {
		return nil
	}
	w.running = false
	close(w.stopCh)
	return nil
}

// Results returns the channel receiving watch results.
// For noopLayerWatcher, this channel never receives any results.
func (w *noopLayerWatcher) Results() <-chan LayerWatchResult {
	return w.results
}
