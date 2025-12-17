package layer_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/yacchi/jubako/format/json"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
)

// testSource is a source that can be updated programmatically for testing.
type testSource struct {
	mu       sync.RWMutex
	data     []byte
	notifyFn watcher.NotifyFunc
}

var _ source.Source = (*testSource)(nil)
var _ source.WatchableSource = (*testSource)(nil)

func (s *testSource) Type() source.SourceType { return "test" }

func newTestSource(data []byte) *testSource {
	return &testSource{data: data}
}

func (s *testSource) Load(ctx context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]byte, len(s.data))
	copy(result, s.data)
	return result, nil
}

func (s *testSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	return source.ErrSaveNotSupported
}

func (s *testSource) CanSave() bool {
	return false
}

func (s *testSource) Watch() (watcher.WatcherInitializer, error) {
	return watcher.NewSubscription(watcher.SubscriptionHandlerFunc(s.subscribe)), nil
}

func (s *testSource) subscribe(ctx context.Context, notify watcher.NotifyFunc) (watcher.StopFunc, error) {
	s.mu.Lock()
	s.notifyFn = notify
	s.mu.Unlock()

	return func(ctx context.Context) error {
		s.mu.Lock()
		s.notifyFn = nil
		s.mu.Unlock()
		return nil
	}, nil
}

// Update updates the source data and notifies watchers.
func (s *testSource) Update(data []byte) {
	s.mu.Lock()
	s.data = data
	notify := s.notifyFn
	s.mu.Unlock()

	if notify != nil {
		notify(data, nil)
	}
}

func TestLayerWatcher_Basic(t *testing.T) {
	initialData := []byte(`{"key": "initial"}`)
	src := newTestSource(initialData)
	l := layer.New("test", src, json.New())

	// Create watcher
	lw, err := l.Watch()
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start watcher
	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer lw.Stop(context.Background())

	// Wait for initial notification or update
	updatedData := []byte(`{"key": "updated"}`)
	go func() {
		time.Sleep(50 * time.Millisecond)
		src.Update(updatedData)
	}()

	// Wait for result
	select {
	case result := <-lw.Results():
		if result.Error != nil {
			t.Fatalf("unexpected error: %v", result.Error)
		}
		if result.Data == nil {
			t.Fatal("expected data, got nil")
		}
		if result.Data["key"] != "updated" {
			t.Errorf("expected key=updated, got %v", result.Data["key"])
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for result")
	}
}

func TestLayerWatcher_MultipleUpdates(t *testing.T) {
	initialData := []byte(`{"count": 0}`)
	src := newTestSource(initialData)
	l := layer.New("test", src, json.New())

	lw, err := l.Watch()
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer lw.Stop(context.Background())

	// Send multiple updates
	updates := []int{1, 2, 3}
	go func() {
		for _, count := range updates {
			time.Sleep(20 * time.Millisecond)
			src.Update([]byte(`{"count": ` + string(rune('0'+count)) + `}`))
		}
	}()

	// Receive updates
	received := 0
	for received < len(updates) {
		select {
		case result := <-lw.Results():
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}
			received++
		case <-ctx.Done():
			t.Fatalf("timeout: received %d/%d updates", received, len(updates))
		}
	}
}

func TestLayerWatcher_Stop(t *testing.T) {
	src := newTestSource([]byte(`{}`))
	l := layer.New("test", src, json.New())

	lw, err := l.Watch()
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	ctx := context.Background()
	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Stop should close the results channel
	if err := lw.Stop(ctx); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	// Channel should be closed
	select {
	case _, ok := <-lw.Results():
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for channel close")
	}
}

func TestLayerWatcher_WithConfigOverride(t *testing.T) {
	src := newTestSource([]byte(`{}`))
	l := layer.New("test", src, json.New())

	// Create watcher with config override
	customInterval := 1 * time.Second
	lw, err := l.Watch(layer.WithLayerWatchConfig(
		watcher.WithPollInterval(customInterval),
	))
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer lw.Stop(context.Background())

	// The watcher should work (config override is applied internally)
	src.Update([]byte(`{"updated": true}`))

	select {
	case result := <-lw.Results():
		if result.Error != nil {
			t.Fatalf("unexpected error: %v", result.Error)
		}
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

// testPollingSource is a source without WatchableSource implementation
// to test fallback polling behavior.
type testPollingSource struct {
	mu   sync.RWMutex
	data []byte
}

var _ source.Source = (*testPollingSource)(nil)

func (s *testPollingSource) Type() source.SourceType { return "test" }

func newTestPollingSource(data []byte) *testPollingSource {
	return &testPollingSource{data: data}
}

func (s *testPollingSource) Load(ctx context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]byte, len(s.data))
	copy(result, s.data)
	return result, nil
}

func (s *testPollingSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	return source.ErrSaveNotSupported
}

func (s *testPollingSource) CanSave() bool {
	return false
}

func (s *testPollingSource) Update(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = data
}

func TestLayerWatcher_FallbackPolling(t *testing.T) {
	initialData := []byte(`{"key": "initial"}`)
	src := newTestPollingSource(initialData)
	l := layer.New("test", src, json.New())

	// Use short poll interval for testing
	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(50 * time.Millisecond))
	lw, err := l.Watch(layer.WithBaseConfig(cfg))
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer lw.Stop(context.Background())

	// First poll should return initial data
	select {
	case result := <-lw.Results():
		if result.Error != nil {
			t.Fatalf("unexpected error: %v", result.Error)
		}
		if result.Data["key"] != "initial" {
			t.Errorf("expected key=initial, got %v", result.Data["key"])
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for initial result")
	}

	// Update source and wait for polling to detect change
	src.Update([]byte(`{"key": "updated"}`))

	select {
	case result := <-lw.Results():
		if result.Error != nil {
			t.Fatalf("unexpected error: %v", result.Error)
		}
		if result.Data["key"] != "updated" {
			t.Errorf("expected key=updated, got %v", result.Data["key"])
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for updated result")
	}
}

// Tests for NoopLayerWatcher

func TestNoopLayerWatcher_Basic(t *testing.T) {
	lw := layer.NewNoopLayerWatcher()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// NoopLayerWatcher should not emit any results
	select {
	case result := <-lw.Results():
		t.Errorf("unexpected result: %+v", result)
	case <-ctx.Done():
		// Expected - no results
	}

	if err := lw.Stop(context.Background()); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

func TestNoopLayerWatcher_Stop(t *testing.T) {
	lw := layer.NewNoopLayerWatcher()

	ctx := context.Background()
	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if err := lw.Stop(ctx); err != nil {
		t.Errorf("Stop() error: %v", err)
	}

	// Results channel should be closed
	select {
	case _, ok := <-lw.Results():
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for channel close")
	}

	// Double stop should not error
	if err := lw.Stop(ctx); err != nil {
		t.Errorf("second Stop() error: %v", err)
	}
}

func TestNoopLayerWatcher_DoubleStart(t *testing.T) {
	lw := layer.NewNoopLayerWatcher()

	ctx := context.Background()

	if err := lw.Start(ctx); err != nil {
		t.Fatalf("first Start() error: %v", err)
	}
	defer lw.Stop(ctx)

	// Second start should be no-op
	if err := lw.Start(ctx); err != nil {
		t.Errorf("second Start() error: %v", err)
	}
}

func TestNoopLayerWatcher_ResultsBeforeStart(t *testing.T) {
	lw := layer.NewNoopLayerWatcher()

	// Results() should return nil before Start() is called
	results := lw.Results()
	if results != nil {
		t.Error("Results() should return nil before Start() is called")
	}
}

func TestNoopLayerWatcher_ContextCancellation(t *testing.T) {
	lw := layer.NewNoopLayerWatcher()

	ctx, cancel := context.WithCancel(context.Background())
	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Cancel the context
	cancel()

	// Results channel should be closed
	select {
	case _, ok := <-lw.Results():
		if ok {
			t.Error("expected channel to be closed after context cancellation")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for channel close")
	}
}

// testErrorWatchSource is a source whose watcher fails to start.
type testErrorWatchSource struct {
	mu   sync.RWMutex
	data []byte
}

var _ source.Source = (*testErrorWatchSource)(nil)
var _ source.WatchableSource = (*testErrorWatchSource)(nil)

func (s *testErrorWatchSource) Type() source.SourceType { return "test" }

func (s *testErrorWatchSource) Load(ctx context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]byte, len(s.data))
	copy(result, s.data)
	return result, nil
}

func (s *testErrorWatchSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	return source.ErrSaveNotSupported
}

func (s *testErrorWatchSource) CanSave() bool {
	return false
}

func (s *testErrorWatchSource) Watch() (watcher.WatcherInitializer, error) {
	return func(params watcher.WatcherInitializerParams) (watcher.Watcher, error) {
		return &errorStartWatcher{}, nil
	}, nil
}

// errorStartWatcher is a watcher that fails to start.
type errorStartWatcher struct{}

func (w *errorStartWatcher) Type() watcher.WatcherType { return "error" }
func (w *errorStartWatcher) Start(ctx context.Context) error {
	return errors.New("start error")
}
func (w *errorStartWatcher) Stop(ctx context.Context) error { return nil }
func (w *errorStartWatcher) Results() <-chan watcher.WatchResult {
	return nil
}

func TestLayerWatcher_StartError(t *testing.T) {
	src := &testErrorWatchSource{data: []byte(`{"key": "value"}`)}
	l := layer.New("test", src, json.New())

	lw, err := l.Watch()
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	err = lw.Start(context.Background())
	if err == nil {
		t.Error("Start() should return error")
	}
}

// errorResultWatchSource is a source whose watcher reports errors in results.
type errorResultWatchSource struct {
	mu       sync.RWMutex
	data     []byte
	notifyFn watcher.NotifyFunc
}

var _ source.Source = (*errorResultWatchSource)(nil)
var _ source.WatchableSource = (*errorResultWatchSource)(nil)

func (s *errorResultWatchSource) Type() source.SourceType { return "test" }

func (s *errorResultWatchSource) Load(ctx context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]byte, len(s.data))
	copy(result, s.data)
	return result, nil
}

func (s *errorResultWatchSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	return source.ErrSaveNotSupported
}

func (s *errorResultWatchSource) CanSave() bool {
	return false
}

func (s *errorResultWatchSource) Watch() (watcher.WatcherInitializer, error) {
	return watcher.NewSubscription(watcher.SubscriptionHandlerFunc(s.subscribe)), nil
}

func (s *errorResultWatchSource) subscribe(ctx context.Context, notify watcher.NotifyFunc) (watcher.StopFunc, error) {
	s.mu.Lock()
	s.notifyFn = notify
	s.mu.Unlock()

	return func(ctx context.Context) error {
		s.mu.Lock()
		s.notifyFn = nil
		s.mu.Unlock()
		return nil
	}, nil
}

func (s *errorResultWatchSource) NotifyError(err error) {
	s.mu.RLock()
	notify := s.notifyFn
	s.mu.RUnlock()

	if notify != nil {
		notify(nil, err)
	}
}

func TestLayerWatcher_ErrorInResults(t *testing.T) {
	src := &errorResultWatchSource{data: []byte(`{"key": "value"}`)}
	l := layer.New("test", src, json.New())

	lw, err := l.Watch()
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer lw.Stop(context.Background())

	// Send an error notification
	go func() {
		time.Sleep(50 * time.Millisecond)
		src.NotifyError(errors.New("test error"))
	}()

	// Should receive the error
	select {
	case result := <-lw.Results():
		if result.Error == nil {
			t.Error("expected error in result")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for error result")
	}
}