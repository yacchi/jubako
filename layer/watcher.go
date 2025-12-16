package layer

import (
	"context"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/source"
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
	// The config is applied at start time.
	Start(ctx context.Context, cfg watcher.WatchConfig) error

	// Stop stops watching and releases resources.
	Stop(ctx context.Context) error

	// Results returns a channel that receives watch results.
	Results() <-chan LayerWatchResult
}

// WatchableLayer is an optional interface that layers can implement
// to support change detection and hot reload.
type WatchableLayer interface {
	// Watch returns a LayerWatcher for this layer.
	// The watcher should not be started yet; the caller will call Start.
	//
	// Options can be used to override the WatchConfig at the layer level.
	Watch(opts ...WatchOption) (LayerWatcher, error)
}

// WatchOption configures layer-level watch behavior.
type WatchOption func(*watchOptions)

type watchOptions struct {
	configOpts []watcher.WatchConfigOption
}

// WithLayerWatchConfig allows layer-level override of WatchConfig.
// These options are applied after Store-level options.
func WithLayerWatchConfig(opts ...watcher.WatchConfigOption) WatchOption {
	return func(o *watchOptions) {
		o.configOpts = append(o.configOpts, opts...)
	}
}

// layerWatcher wraps a source-level watcher and transforms []byte to map[string]any.
type layerWatcher struct {
	watcher    watcher.Watcher
	doc        document.Document
	configOpts []watcher.WatchConfigOption
	results    chan LayerWatchResult
}

func newLayerWatcher(w watcher.Watcher, doc document.Document, opts []watcher.WatchConfigOption) *layerWatcher {
	return &layerWatcher{
		watcher:    w,
		doc:        doc,
		configOpts: opts,
	}
}

// Start begins watching and transforms results from []byte to map[string]any.
func (w *layerWatcher) Start(ctx context.Context, cfg watcher.WatchConfig) error {
	w.results = make(chan LayerWatchResult)

	// Apply layer-level options to override store-level config
	cfg.ApplyOptions(w.configOpts...)

	if err := w.watcher.Start(ctx, cfg); err != nil {
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

// fallbackPollHandler implements watcher.PollHandler for sources
// that don't implement WatchableSource.
type fallbackPollHandler struct {
	source source.Source
}

func newFallbackPollHandler(src source.Source) *fallbackPollHandler {
	return &fallbackPollHandler{source: src}
}

// Poll loads data from the source.
// The PollingWatcher will use CompareFunc to detect changes.
func (h *fallbackPollHandler) Poll(ctx context.Context) ([]byte, error) {
	return h.source.Load(ctx)
}
