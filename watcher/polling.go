package watcher

import (
	"context"
	"sync"
	"time"
)

// PollHandler defines the interface for polling-based change detection.
// Implementations should fetch the latest data and indicate if it changed.
type PollHandler interface {
	// Poll fetches the latest data from the source.
	// Returns the data and any error encountered.
	// The watcher will use CompareFunc to detect changes.
	Poll(ctx context.Context) (data []byte, err error)
}

// PollHandlerFunc is a function that implements PollHandler.
type PollHandlerFunc func(ctx context.Context) (data []byte, err error)

// Poll implements PollHandler.
func (f PollHandlerFunc) Poll(ctx context.Context) ([]byte, error) {
	return f(ctx)
}

// pollingWatcher implements Watcher using polling.
type pollingWatcher struct {
	handler PollHandler

	results  chan WatchResult
	stopCh   chan struct{}
	lastData []byte

	mu      sync.Mutex
	running bool
}

// NewPolling creates a new polling-based Watcher.
// The handler's Poll method is called at each interval.
// Changes are detected using the CompareFunc from WatchConfig.
func NewPolling(handler PollHandler) Watcher {
	return &pollingWatcher{
		handler: handler,
	}
}

// Type returns the watcher type identifier.
func (w *pollingWatcher) Type() WatcherType {
	return TypePolling
}

// Start begins polling at the configured interval.
// The first poll happens immediately.
func (w *pollingWatcher) Start(ctx context.Context, cfg WatchConfig) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.results = make(chan WatchResult)
	w.stopCh = make(chan struct{})
	w.mu.Unlock()

	compareFunc := cfg.CompareFunc
	if compareFunc == nil {
		compareFunc = DefaultCompareFunc
	}

	interval := cfg.PollInterval
	if interval <= 0 {
		interval = DefaultPollInterval
	}

	go func() {
		defer close(w.results)

		for {
			startTime := time.Now()

			data, err := w.handler.Poll(ctx)
			if err != nil {
				select {
				case w.results <- WatchResult{Error: err}:
				case <-ctx.Done():
					return
				case <-w.stopCh:
					return
				}
			} else if w.lastData == nil || compareFunc(w.lastData, data) {
				// First poll or data changed
				w.lastData = data
				select {
				case w.results <- WatchResult{Data: data}:
				case <-ctx.Done():
					return
				case <-w.stopCh:
					return
				}
			}

			// Calculate wait time, accounting for processing time
			elapsed := time.Since(startTime)
			waitTime := interval - elapsed
			if waitTime <= 0 {
				// Processing took longer than interval, skip wait
				continue
			}

			select {
			case <-time.After(waitTime):
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			}
		}
	}()

	return nil
}

// Stop stops polling.
func (w *pollingWatcher) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}
	w.running = false
	close(w.stopCh)
	return nil
}

// Results returns the channel receiving poll results.
func (w *pollingWatcher) Results() <-chan WatchResult {
	return w.results
}
