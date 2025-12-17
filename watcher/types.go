// Package watcher provides abstractions for watching configuration changes.
// It supports both polling-based and subscription-based (event-driven) change detection.
package watcher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"github.com/yacchi/jubako/types"
)

// Errors for WatcherInitializerParams validation.
var (
	// ErrFetchRequired is returned when Fetch is nil in WatcherInitializerParams.
	ErrFetchRequired = errors.New("watcher: Fetch is required")

	// ErrOpMuRequired is returned when OpMu is nil in WatcherInitializerParams.
	ErrOpMuRequired = errors.New("watcher: OpMu is required")
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

	// CompareFunc is used by PollingWatcher to detect changes between
	// the previous poll result and the current one. When the poll function
	// returns changed=true with data, the watcher uses CompareFunc to
	// determine if the data actually differs from the last known data.
	// This allows sources to always return changed=true (indicating successful fetch)
	// while the watcher handles the actual change detection.
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

// ApplyDefaults fills zero/nil values with defaults.
// This ensures that after all options are applied, the config has valid values.
// Call this after ApplyOptions to guarantee no zero/nil values remain.
func (c *WatchConfig) ApplyDefaults() {
	if c.PollInterval <= 0 {
		c.PollInterval = DefaultPollInterval
	}
	if c.CompareFunc == nil {
		c.CompareFunc = DefaultCompareFunc
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
//
// The callback follows this convention:
//   - notify(data, nil): Data is available from the source (push-style).
//   - notify(nil, err): An error occurred during watching.
//   - notify(nil, nil): A change was detected but data must be fetched separately.
//
// When notify(nil, nil) is called, the subscriber should use FetchFunc to get the data.
type NotifyFunc func(data []byte, err error)

// FetchFunc fetches the latest data from a source and reports whether it changed.
// This is a unified function type used by both polling and subscription watchers.
//
// Return value semantics:
//   - (true, data, nil): Data changed and is available.
//   - (false, nil, nil): No change detected (e.g., 304 Not Modified, same content).
//   - (false, nil, err): An error occurred.
//
// For subscription watchers, when NotifyFunc is called with (nil, nil),
// the watcher calls FetchFunc to get the data. If FetchFunc returns changed=false,
// no notification is emitted to downstream consumers.
type FetchFunc func(ctx context.Context) (changed bool, data []byte, err error)

// WatcherInitializerParams contains parameters for WatcherInitializer.
// All fields are required and will be validated by the initializer.
type WatcherInitializerParams struct {
	// Fetch is a function to fetch the latest data and detect changes.
	// Used by subscription-based watchers when event-only notification (nil, nil) is received.
	// Required: must not be nil.
	Fetch FetchFunc

	// OpMu is a mutex for operation-level synchronization.
	// Both polling and subscription watchers will wrap their fetch operations with this mutex
	// to ensure mutual exclusion with Load/Save operations.
	// Required: must not be nil.
	OpMu *sync.Mutex

	// Config contains watch configuration (PollInterval, CompareFunc, etc.).
	// This allows configuration to be finalized at initialization time,
	// eliminating the need for synchronization in Start().
	Config WatchConfig
}

// Validate checks that all required fields are set.
func (p WatcherInitializerParams) Validate() error {
	if p.Fetch == nil {
		return ErrFetchRequired
	}
	if p.OpMu == nil {
		return ErrOpMuRequired
	}
	return nil
}

// WatcherInitializer is a factory function that creates a Watcher.
// Sources return this from Watch() to defer actual watcher creation to the caller.
// This allows the caller (typically Layer) to provide synchronized fetch functions
// and operation mutex without the source needing to know about synchronization details.
//
// The params argument contains all required parameters. Implementations should
// call params.Validate() to ensure all required fields are set.
type WatcherInitializer func(params WatcherInitializerParams) (Watcher, error)

// StopFunc stops a subscription.
// The context can be used for timeout/cancellation of cleanup operations.
type StopFunc func(ctx context.Context) error

// wrapFetchWithMutex wraps a FetchFunc with mutex protection if opMu is non-nil.
// This is used by NewPolling and NewSubscription to ensure mutual exclusion
// with Load/Save operations.
func wrapFetchWithMutex(fetch FetchFunc, opMu *sync.Mutex) FetchFunc {
	if opMu == nil || fetch == nil {
		return fetch
	}
	return func(ctx context.Context) (bool, []byte, error) {
		opMu.Lock()
		defer opMu.Unlock()
		return fetch(ctx)
	}
}
