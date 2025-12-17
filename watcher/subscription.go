package watcher

import (
	"context"
	"sync"
)

// SubscriptionHandler defines the interface for subscription-based change detection.
// Implementations register for notifications and call the notify function when data changes.
type SubscriptionHandler interface {
	// Subscribe starts receiving change notifications.
	// The notify function should be called when data changes or an error occurs.
	// Returns a StopFunc to unsubscribe, or an error if subscription failed.
	Subscribe(ctx context.Context, notify NotifyFunc) (StopFunc, error)
}

// SubscriptionHandlerFunc is a function that implements SubscriptionHandler.
type SubscriptionHandlerFunc func(ctx context.Context, notify NotifyFunc) (StopFunc, error)

// Subscribe implements SubscriptionHandler.
func (f SubscriptionHandlerFunc) Subscribe(ctx context.Context, notify NotifyFunc) (StopFunc, error) {
	return f(ctx, notify)
}

// subscriptionWatcher implements Watcher using subscriptions.
type subscriptionWatcher struct {
	handler SubscriptionHandler
	fetcher FetchFunc

	results chan WatchResult
	stopCh  chan struct{}
	stopFn  StopFunc

	mu      sync.Mutex
	running bool
}

// newSubscriptionWatcher creates a subscriptionWatcher with the given handler and fetch function.
// This is an internal constructor used by both NewSubscription and NewPolling.
func newSubscriptionWatcher(handler SubscriptionHandler, fetch FetchFunc, opMu *sync.Mutex) *subscriptionWatcher {
	return &subscriptionWatcher{
		handler: handler,
		fetcher: wrapFetchWithMutex(fetch, opMu),
	}
}

// NewSubscription returns a WatcherInitializer that creates a subscription-based Watcher.
// The handler's Subscribe method sets up event-based notifications.
//
// When the initializer is called with params.Fetch, it becomes the fetcher used
// when the handler notifies with (nil, nil) - indicating a change was detected
// but data must be fetched separately. If FetchFunc returns changed=false,
// no notification is emitted to downstream consumers.
//
// The fetch function will be wrapped with mutex protection using params.OpMu
// to ensure mutual exclusion with Load/Save operations.
func NewSubscription(handler SubscriptionHandler) WatcherInitializer {
	return func(params WatcherInitializerParams) (Watcher, error) {
		if err := params.Validate(); err != nil {
			return nil, err
		}
		return newSubscriptionWatcher(handler, params.Fetch, params.OpMu), nil
	}
}

// Type returns the watcher type identifier.
func (w *subscriptionWatcher) Type() WatcherType {
	return TypeSubscription
}

// Start begins the subscription.
// Configuration is provided at initialization time via WatcherInitializerParams.
func (w *subscriptionWatcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.results = make(chan WatchResult)
	w.stopCh = make(chan struct{})
	w.mu.Unlock()

	stopCh := w.stopCh // capture for closure

	notify := func(data []byte, err error) {
		// Handle the three notification patterns:
		// 1. notify(data, nil): Push-style, data is already available
		// 2. notify(nil, err): Error occurred
		// 3. notify(nil, nil): Event-only, need to fetch data
		if data == nil && err == nil {
			// Event-only notification: fetch data using the synchronized fetcher
			if w.fetcher == nil {
				// No fetcher available for event-only notification.
				// This is a configuration error - event-only sources must provide
				// a fetch function via WatcherInitializerParams.Fetch.
				// Skip notification to avoid sending nil data.
				return
			}
			changed, fetchedData, fetchErr := w.fetcher(ctx)
			if !changed && fetchErr == nil {
				// No change detected, skip notification
				return
			}
			data, err = fetchedData, fetchErr
		}

		select {
		case w.results <- WatchResult{Data: data, Error: err}:
		case <-ctx.Done():
		case <-stopCh:
		}
	}

	stop, err := w.handler.Subscribe(ctx, notify)
	if err != nil {
		w.mu.Lock()
		w.running = false
		close(w.stopCh)
		close(w.results)
		w.mu.Unlock()
		return err
	}

	w.mu.Lock()
	w.stopFn = stop
	w.mu.Unlock()

	return nil
}

// Stop stops the subscription.
func (w *subscriptionWatcher) Stop(ctx context.Context) error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = false

	// Signal stop to unblock notify
	close(w.stopCh)
	w.mu.Unlock()

	// Call handler's stop function outside of lock
	var err error
	if w.stopFn != nil {
		err = w.stopFn(ctx)
	}

	w.mu.Lock()
	w.stopFn = nil
	close(w.results)
	w.mu.Unlock()

	return err
}

// Results returns the channel receiving subscription results.
func (w *subscriptionWatcher) Results() <-chan WatchResult {
	return w.results
}
