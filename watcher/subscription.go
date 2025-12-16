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

	results chan WatchResult
	stopFn  StopFunc

	mu      sync.Mutex
	running bool
}

// NewSubscription creates a new subscription-based Watcher.
// The handler's Subscribe method sets up event-based notifications.
func NewSubscription(handler SubscriptionHandler) Watcher {
	return &subscriptionWatcher{
		handler: handler,
	}
}

// Type returns the watcher type identifier.
func (w *subscriptionWatcher) Type() WatcherType {
	return TypeSubscription
}

// Start begins the subscription.
// WatchConfig is accepted for interface compatibility but most fields
// are not used by subscription-based watchers (e.g., PollInterval is ignored).
func (w *subscriptionWatcher) Start(ctx context.Context, cfg WatchConfig) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.results = make(chan WatchResult)
	w.mu.Unlock()

	notify := func(data []byte, err error) {
		select {
		case w.results <- WatchResult{Data: data, Error: err}:
		case <-ctx.Done():
		}
	}

	stop, err := w.handler.Subscribe(ctx, notify)
	if err != nil {
		w.mu.Lock()
		w.running = false
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
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}
	w.running = false

	var err error
	if w.stopFn != nil {
		err = w.stopFn(ctx)
		w.stopFn = nil
	}

	close(w.results)
	return err
}

// Results returns the channel receiving subscription results.
func (w *subscriptionWatcher) Results() <-chan WatchResult {
	return w.results
}
