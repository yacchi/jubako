package jubako_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/format/json"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/source/bytes"
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

func (s *testSource) Watch() (watcher.Watcher, error) {
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

type TestConfig struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

func TestStore_Watch_Basic(t *testing.T) {
	src := newTestSource([]byte(`{"value": "initial", "count": 0}`))

	store := jubako.New[TestConfig]()
	if err := store.Add(layer.New("test", src, json.New())); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Verify initial value
	cfg := store.Get()
	if cfg.Value != "initial" {
		t.Errorf("expected value=initial, got %s", cfg.Value)
	}

	// Track subscriber calls
	var subscriberCalled atomic.Int32
	var lastValue atomic.Value
	store.Subscribe(func(cfg TestConfig) {
		subscriberCalled.Add(1)
		lastValue.Store(cfg.Value)
	})

	// Start watching
	watchCfg := jubako.StoreWatchConfig{
		DebounceDelay: 10 * time.Millisecond,
	}
	stop, err := store.Watch(ctx, watchCfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	defer stop(context.Background())

	// Update source
	src.Update([]byte(`{"value": "updated", "count": 1}`))

	// Wait for subscriber to be called
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v := lastValue.Load(); v != nil && v.(string) == "updated" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Verify updated value
	cfg = store.Get()
	if cfg.Value != "updated" {
		t.Errorf("expected value=updated, got %s", cfg.Value)
	}
	if cfg.Count != 1 {
		t.Errorf("expected count=1, got %d", cfg.Count)
	}
}

func TestStore_Watch_MultipleUpdates(t *testing.T) {
	src := newTestSource([]byte(`{"value": "v0", "count": 0}`))

	store := jubako.New[TestConfig]()
	if err := store.Add(layer.New("test", src, json.New())); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	var subscriberCalls atomic.Int32
	store.Subscribe(func(cfg TestConfig) {
		subscriberCalls.Add(1)
	})

	watchCfg := jubako.StoreWatchConfig{
		DebounceDelay: 50 * time.Millisecond,
	}
	stop, err := store.Watch(ctx, watchCfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	defer stop(context.Background())

	// Send rapid updates - should be debounced
	for i := 1; i <= 5; i++ {
		src.Update([]byte(`{"value": "v` + string(rune('0'+i)) + `", "count": ` + string(rune('0'+i)) + `}`))
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce to complete
	time.Sleep(100 * time.Millisecond)

	// Due to debouncing, we should have fewer subscriber calls than updates
	cfg := store.Get()
	if cfg.Value != "v5" {
		t.Errorf("expected final value=v5, got %s", cfg.Value)
	}
}

func TestStore_Watch_WithNoWatch(t *testing.T) {
	src1 := newTestSource([]byte(`{"value": "watched"}`))
	src2 := bytes.FromString(`{"count": 100}`)

	store := jubako.New[TestConfig]()
	if err := store.Add(layer.New("watched", src1, json.New())); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	// Add with NoWatch - this layer should not trigger reloads
	if err := store.Add(layer.New("static", src2, json.New()), jubako.WithNoWatch()); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	watchCfg := jubako.StoreWatchConfig{
		DebounceDelay: 10 * time.Millisecond,
	}
	stop, err := store.Watch(ctx, watchCfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	defer stop(context.Background())

	// Update watched source
	var lastValue atomic.Value
	store.Subscribe(func(cfg TestConfig) {
		lastValue.Store(cfg.Value)
	})

	src1.Update([]byte(`{"value": "changed"}`))

	// Wait for update
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v := lastValue.Load(); v != nil && v.(string) == "changed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cfg := store.Get()
	if cfg.Value != "changed" {
		t.Errorf("expected value=changed, got %s", cfg.Value)
	}
}

func TestStore_Watch_OnReloadCallback(t *testing.T) {
	src := newTestSource([]byte(`{"value": "initial"}`))

	store := jubako.New[TestConfig]()
	if err := store.Add(layer.New("test", src, json.New())); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	var onReloadCalled atomic.Int32
	watchCfg := jubako.StoreWatchConfig{
		DebounceDelay: 10 * time.Millisecond,
		OnReload: func() {
			onReloadCalled.Add(1)
		},
	}

	stop, err := store.Watch(ctx, watchCfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	defer stop(context.Background())

	src.Update([]byte(`{"value": "updated"}`))

	// Wait for OnReload
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if onReloadCalled.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if onReloadCalled.Load() == 0 {
		t.Error("OnReload was not called")
	}
}

func TestStore_Watch_OnErrorCallback(t *testing.T) {
	// Use a source that will return an error when parsing
	src := newTestSource([]byte(`{"value": "valid"}`))

	store := jubako.New[TestConfig]()
	if err := store.Add(layer.New("test", src, json.New())); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	var onErrorCalled atomic.Int32
	watchCfg := jubako.StoreWatchConfig{
		DebounceDelay: 10 * time.Millisecond,
		OnError: func(name layer.Name, err error) {
			onErrorCalled.Add(1)
		},
	}

	stop, err := store.Watch(ctx, watchCfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	defer stop(context.Background())

	// Send invalid JSON to trigger parse error
	src.Update([]byte(`{invalid json`))

	// Wait for OnError
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if onErrorCalled.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if onErrorCalled.Load() == 0 {
		t.Error("OnError was not called for invalid JSON")
	}
}

func TestStore_Watch_Stop(t *testing.T) {
	src := newTestSource([]byte(`{"value": "initial"}`))

	store := jubako.New[TestConfig]()
	if err := store.Add(layer.New("test", src, json.New())); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	ctx := context.Background()
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	watchCfg := jubako.DefaultStoreWatchConfig()
	stop, err := store.Watch(ctx, watchCfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	// Stop should complete without error
	if err := stop(ctx); err != nil {
		t.Errorf("Stop() error: %v", err)
	}

	// Updates after stop should not cause issues
	src.Update([]byte(`{"value": "after-stop"}`))
	time.Sleep(50 * time.Millisecond)

	// Value should not have changed
	cfg := store.Get()
	if cfg.Value != "initial" {
		t.Errorf("expected value=initial after stop, got %s", cfg.Value)
	}
}

func TestStore_Watch_NoWatchableLayers(t *testing.T) {
	// bytes.Source has NoopWatcher, so effectively no watchable layers
	src := bytes.FromString(`{"value": "static"}`)

	store := jubako.New[TestConfig]()
	if err := store.Add(layer.New("static", src, json.New()), jubako.WithNoWatch()); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	ctx := context.Background()
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	watchCfg := jubako.DefaultStoreWatchConfig()
	stop, err := store.Watch(ctx, watchCfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	// Should return without error even with no watchable layers
	if err := stop(ctx); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

// Ensure testSource implements the document interface check
var _ document.Document = json.New()
