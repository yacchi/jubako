package watcher

import "context"

// Watcher watches for changes and notifies via a channel.
// Implementations include PollingWatcher, SubscriptionWatcher, and NoopWatcher.
type Watcher interface {
	// Type returns the watcher type identifier (e.g., TypePolling, TypeSubscription, TypeNoop).
	// This is used for introspection and debugging.
	Type() WatcherType

	// Start begins watching for changes.
	// The config is applied at start time, allowing runtime configuration.
	// Results are sent to the channel returned by Results().
	// Must be called before Results() returns a valid channel.
	Start(ctx context.Context, cfg WatchConfig) error

	// Stop stops watching and releases resources.
	// The context can be used for timeout/cancellation of cleanup operations.
	// After Stop returns, no more results will be sent.
	Stop(ctx context.Context) error

	// Results returns a channel that receives watch results.
	// The channel is created by Start and closed by Stop.
	// Returns nil if Start has not been called.
	Results() <-chan WatchResult
}
