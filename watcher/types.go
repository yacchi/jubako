// Package watcher provides abstractions for watching configuration changes.
// It supports both polling-based and subscription-based (event-driven) change detection.
package watcher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"time"

	"github.com/yacchi/jubako/types"
)

// DefaultPollInterval is the default polling interval for change detection.
const DefaultPollInterval = 30 * time.Second

// WatcherType is an alias for types.WatcherType.
// This allows watcher implementations to use watcher.WatcherType directly.
type WatcherType = types.WatcherType

// Standard watcher types.
const (
	// TypePolling is a watcher that polls at regular intervals.
	TypePolling WatcherType = "polling"

	// TypeSubscription is an event-based watcher (e.g., fsnotify).
	TypeSubscription WatcherType = "subscription"

	// TypeNoop is a watcher that never fires (for immutable sources).
	TypeNoop WatcherType = "noop"
)

// CompareFunc compares two byte slices and returns true if they are different.
type CompareFunc func(old, new []byte) bool

// DefaultCompareFunc compares byte slices directly using bytes.Equal.
// This is efficient for small to medium-sized data.
func DefaultCompareFunc(old, new []byte) bool {
	return !bytes.Equal(old, new)
}

// HashCompareFunc compares byte slices using SHA-256 hashes.
// This is more efficient for large data where keeping a copy is expensive.
func HashCompareFunc(old, new []byte) bool {
	return sha256.Sum256(old) != sha256.Sum256(new)
}

// WatchConfig configures watcher behavior.
type WatchConfig struct {
	// PollInterval is the interval between polling attempts.
	// Only used by PollingWatcher. Default is 30 seconds.
	PollInterval time.Duration

	// CompareFunc is used to detect changes between old and new data.
	// Default is DefaultCompareFunc (bytes.Equal).
	CompareFunc CompareFunc
}

// WatchConfigOption is a functional option for WatchConfig.
type WatchConfigOption func(*WatchConfig)

// WithPollInterval sets the polling interval.
func WithPollInterval(d time.Duration) WatchConfigOption {
	return func(c *WatchConfig) {
		c.PollInterval = d
	}
}

// WithCompareFunc sets the comparison function for change detection.
func WithCompareFunc(f CompareFunc) WatchConfigOption {
	return func(c *WatchConfig) {
		c.CompareFunc = f
	}
}

// NewWatchConfig creates a WatchConfig with the given options.
// Defaults: PollInterval=30s, CompareFunc=DefaultCompareFunc.
func NewWatchConfig(opts ...WatchConfigOption) WatchConfig {
	cfg := WatchConfig{
		PollInterval: DefaultPollInterval,
		CompareFunc:  DefaultCompareFunc,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// ApplyOptions applies the given options to the config.
func (c *WatchConfig) ApplyOptions(opts ...WatchConfigOption) {
	for _, opt := range opts {
		opt(c)
	}
}

// WatchResult represents the result of a watch cycle.
type WatchResult struct {
	// Data is the latest data from the source.
	// Only set when a change is detected or on initial load.
	Data []byte

	// Error is set if the watch encountered an error.
	Error error
}

// NotifyFunc is a callback for subscription-based watchers.
// Called when data changes or an error occurs.
type NotifyFunc func(data []byte, err error)

// StopFunc stops a subscription.
// The context can be used for timeout/cancellation of cleanup operations.
type StopFunc func(ctx context.Context) error
