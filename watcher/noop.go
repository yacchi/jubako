package watcher

import (
	"context"
	"sync"
)

// noopWatcher implements Watcher but never reports changes.
// This is used for sources that don't change (e.g., bytes.Source).
type noopWatcher struct {
	results chan WatchResult
	stopCh  chan struct{}

	mu      sync.Mutex
	running bool
}

// NewNoop returns a WatcherInitializer that creates a Watcher that never reports changes.
// This is useful for immutable sources like bytes.Source to explicitly
// indicate that watching is not needed, rather than returning an error.
//
// Note: While params are validated for consistency, noop watchers never actually
// use the Fetch or OpMu values since they don't fetch data or need synchronization.
func NewNoop() WatcherInitializer {
	return func(params WatcherInitializerParams) (Watcher, error) {
		if err := params.Validate(); err != nil {
			return nil, err
		}
		// Both Fetch and OpMu are validated but not used - noop watcher never uses them
		return &noopWatcher{}, nil
	}
}

// Type returns the watcher type identifier.
func (w *noopWatcher) Type() WatcherType {
	return TypeNoop
}

// Start begins the noop watcher.
// The watcher will block until Stop is called, but never emit results.
// Configuration is provided at initialization time via WatcherInitializerParams,
// but noop watchers don't use any configuration.
func (w *noopWatcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.results = make(chan WatchResult)
	w.stopCh = make(chan struct{})
	w.mu.Unlock()

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
func (w *noopWatcher) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}
	w.running = false
	close(w.stopCh)
	return nil
}

// Results returns the channel receiving watch results.
// For NoopWatcher, this channel never receives any results.
func (w *noopWatcher) Results() <-chan WatchResult {
	return w.results
}
