package watcher

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// pollingWatcher embeds subscriptionWatcher and implements SubscriptionHandler.
// It acts as its own handler, running the polling loop in Subscribe.
type pollingWatcher struct {
	*subscriptionWatcher
	poll FetchFunc

	// Configuration is set at initialization time and never changes
	interval    time.Duration
	compareFunc CompareFunc

	// Runtime state protected by mutex
	mu       sync.Mutex
	lastData []byte
}

var _ SubscriptionHandler = (*pollingWatcher)(nil)

// NewPolling returns a WatcherInitializer that creates a polling-based Watcher.
//
// The poll function is called at each interval (configured via WatcherInitializerParams.Config.PollInterval).
// The first poll happens immediately.
//
// The poll function will be wrapped with mutex protection using params.OpMu
// to ensure mutual exclusion with Load/Save operations.
//
// Return values for the poll function:
//   - changed=false: No notification is emitted
//   - changed=true with data: Notification is emitted with the data
//   - changed=true with data=nil: Event-only; the watcher uses the injected FetchFunc
//     (provided via params.Fetch) to fetch the actual data
//   - err!=nil: Error notification is emitted
//
// Note: params.Config is expected to have valid values (non-zero PollInterval, non-nil CompareFunc).
// Use layer.ResolveWatchConfig() or WatchConfig.ApplyDefaults() to ensure this.
func NewPolling(poll FetchFunc) WatcherInitializer {
	return func(params WatcherInitializerParams) (Watcher, error) {
		if err := params.Validate(); err != nil {
			return nil, err
		}

		pw := &pollingWatcher{
			poll:        wrapFetchWithMutex(poll, params.OpMu),
			interval:    params.Config.PollInterval,
			compareFunc: params.Config.CompareFunc,
		}
		pw.subscriptionWatcher = newSubscriptionWatcher(pw, params.Fetch, params.OpMu)
		return pw, nil
	}
}

// Type returns TypePolling.
func (w *pollingWatcher) Type() WatcherType {
	return TypePolling
}

// Subscribe implements SubscriptionHandler. It starts the polling loop.
// The polling loop uses CompareFunc to detect changes between consecutive polls.
func (w *pollingWatcher) Subscribe(ctx context.Context, notify NotifyFunc) (StopFunc, error) {
	stopCh := make(chan struct{})
	var wg sync.WaitGroup
	var once sync.Once

	wg.Add(1)
	go func() {
		defer wg.Done()

		var polling int32 // 0 = not polling, 1 = polling

		runOnce := func() {
			// Skip if already polling (prevent double execution)
			if !atomic.CompareAndSwapInt32(&polling, 0, 1) {
				return
			}
			defer atomic.StoreInt32(&polling, 0)

			changed, data, err := w.poll(ctx)
			if err != nil {
				notify(nil, err)
				return
			}

			// Use CompareFunc to detect actual changes
			if changed {
				// Event-only notification: fetch data using the synchronized fetcher
				if data == nil && w.fetcher != nil {
					var fetchErr error
					changed, data, fetchErr = w.fetcher(ctx)
					if fetchErr != nil {
						notify(nil, fetchErr)
						return
					}
					if !changed {
						// Fetcher says no change, skip notification
						return
					}
				}

				w.mu.Lock()
				lastData := w.lastData
				if w.compareFunc(lastData, data) {
					// Data has changed
					w.lastData = data
					w.mu.Unlock()
					notify(data, nil)
				} else {
					// No change detected
					w.mu.Unlock()
				}
			}
		}

		// First poll happens immediately.
		runOnce()

		for {
			startTime := time.Now()

			// Wait for interval, accounting for poll execution time
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-time.After(w.interval):
			}

			runOnce()

			// Adjust next wait to maintain consistent interval
			elapsed := time.Since(startTime)
			if elapsed >= w.interval {
				// Poll took longer than interval, skip additional wait
				continue
			}
		}
	}()

	stop := func(ctx context.Context) error {
		once.Do(func() { close(stopCh) })
		wg.Wait()

		// Reset state for potential restart
		w.mu.Lock()
		w.lastData = nil
		w.mu.Unlock()

		return nil
	}

	return stop, nil
}
