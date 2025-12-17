package jubako

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/watcher"
)

// StoreWatchConfig configures the Watch behavior.
type StoreWatchConfig struct {
	// DebounceDelay is the delay to wait for additional changes before triggering reload.
	// This helps batch rapid successive changes into a single reload.
	// Default: 100ms
	DebounceDelay time.Duration

	// OnError is called when a watch error occurs.
	// layerName may be empty for store-level errors (e.g., materialization failures).
	// If nil, errors are silently ignored.
	OnError func(layerName layer.Name, err error)

	// OnReload is called after a successful reload.
	// This is called in addition to any registered subscribers.
	OnReload func()

	// WatcherOpts are options applied to all layer watchers.
	// These options are applied to the WatchConfig used by Store.Watch.
	WatcherOpts []watcher.WatchConfigOption
}

// DefaultStoreWatchConfig returns the default watch configuration.
func DefaultStoreWatchConfig() StoreWatchConfig {
	return StoreWatchConfig{
		DebounceDelay: 100 * time.Millisecond,
		WatcherOpts: []watcher.WatchConfigOption{
			watcher.WithPollInterval(30 * time.Second),
		},
	}
}

// layerWatchState holds the state for a single layer's watcher.
type layerWatchState struct {
	name    layer.Name
	watcher layer.LayerWatcher
	entry   *layerEntry
}

// layerUpdate represents an update from a single layer's watcher.
type layerUpdate struct {
	name   layer.Name
	entry  *layerEntry
	result layer.LayerWatchResult
}

// Watch starts watching all watchable layers for changes.
// When changes are detected, the store automatically reloads and notifies subscribers.
// Call Load before Watch to materialize an initial configuration snapshot.
//
// Layers marked with WithNoWatch are skipped.
// Returns a stop function to stop watching all layers.
//
// Example:
//
//	stop, err := store.Watch(ctx, jubako.DefaultStoreWatchConfig())
//	if err != nil {
//	  log.Fatal(err)
//	}
//	defer stop(context.Background())
func (s *Store[T]) Watch(ctx context.Context, cfg StoreWatchConfig) (stop func(context.Context) error, err error) {
	s.mu.Lock()

	// Build WatchConfig from options
	watchCfg := watcher.NewWatchConfig(cfg.WatcherOpts...)

	// Collect watchable layers
	var watchers []layerWatchState
	for _, entry := range s.layers {
		// Skip layers that have watching disabled
		if entry.noWatch {
			continue
		}

		// Create the watcher with config (doesn't start yet)
		// The watchCfg is passed via WithBaseConfig option
		lw, err := entry.layer.Watch(layer.WithBaseConfig(watchCfg))
		if err != nil {
			s.mu.Unlock()
			// Clean up any watchers we already created
			for _, ws := range watchers {
				ws.watcher.Stop(ctx)
			}
			return nil, fmt.Errorf("failed to create watcher for layer %q: %w", entry.layer.Name(), err)
		}

		watchers = append(watchers, layerWatchState{
			name:    entry.layer.Name(),
			watcher: lw,
			entry:   entry,
		})
	}
	s.mu.Unlock()

	if len(watchers) == 0 {
		// No watchable layers - return a no-op stop function
		return func(ctx context.Context) error { return nil }, nil
	}

	// Start all watchers and merge their results
	merged := make(chan layerUpdate, len(watchers)*10)
	watchCtx, watchCancel := context.WithCancel(ctx)

	for _, ws := range watchers {
		// Configuration was already provided at Watch() time via WatcherInitializerParams
		if err := ws.watcher.Start(watchCtx); err != nil {
			watchCancel()
			// Clean up watchers
			for _, w := range watchers {
				w.watcher.Stop(ctx)
			}
			return nil, fmt.Errorf("failed to start watcher for layer %q: %w", ws.name, err)
		}

		// Forward results to merged channel
		// Note: Since Go 1.22, loop variables are scoped per-iteration,
		// so capturing 'ws' directly in the closure is safe.
		go func() {
			for result := range ws.watcher.Results() {
				select {
				case merged <- layerUpdate{name: ws.name, entry: ws.entry, result: result}:
				case <-watchCtx.Done():
					return
				}
			}
		}()
	}

	// Process merged updates with debouncing
	go s.watchLoop(watchCtx, merged, cfg)

	// Return stop function
	stop = func(stopCtx context.Context) error {
		watchCancel()

		var errs []error
		for _, ws := range watchers {
			if err := ws.watcher.Stop(stopCtx); err != nil {
				errs = append(errs, fmt.Errorf("failed to stop watcher for layer %q: %w", ws.name, err))
			}
		}

		close(merged)
		return errors.Join(errs...)
	}

	return stop, nil
}

// watchLoop processes updates with debouncing.
func (s *Store[T]) watchLoop(ctx context.Context, updates <-chan layerUpdate, cfg StoreWatchConfig) {
	var debounceTimer *time.Timer
	var pendingUpdates map[layer.Name]layerUpdate

	resetDebounce := func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.NewTimer(cfg.DebounceDelay)
	}

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case update, ok := <-updates:
			if !ok {
				return
			}

			// Handle errors immediately
			if update.result.Error != nil {
				if cfg.OnError != nil {
					cfg.OnError(update.name, update.result.Error)
				}
				continue
			}

			// Initialize pending updates map if needed
			if pendingUpdates == nil {
				pendingUpdates = make(map[layer.Name]layerUpdate)
			}

			// Keep the latest update for each layer
			pendingUpdates[update.name] = update

			// Reset debounce timer
			resetDebounce()

		case <-func() <-chan time.Time {
			if debounceTimer != nil {
				return debounceTimer.C
			}
			// Return a channel that never fires
			return make(chan time.Time)
		}():
			// Debounce period elapsed - apply all pending updates
			if len(pendingUpdates) > 0 {
				s.applyUpdates(ctx, pendingUpdates, cfg)
				pendingUpdates = nil
			}
		}
	}
}

// applyUpdates applies the pending layer updates and triggers reload.
func (s *Store[T]) applyUpdates(ctx context.Context, updates map[layer.Name]layerUpdate, cfg StoreWatchConfig) {
	s.mu.Lock()

	// Update layer data from watchers
	for _, update := range updates {
		update.entry.data = update.result.Data
		// Clear changeset as we have fresh data
		update.entry.changeset = nil
		update.entry.dirty = false
	}

	// Re-materialize the configuration
	current, subscribers, err := s.materializeLocked()
	s.mu.Unlock()

	if err != nil {
		if cfg.OnError != nil {
			cfg.OnError("", fmt.Errorf("failed to materialize after watch update: %w", err))
		}
		return
	}

	// Notify subscribers
	for _, sub := range subscribers {
		sub.fn(current)
	}

	// Call OnReload callback
	if cfg.OnReload != nil {
		cfg.OnReload()
	}
}
