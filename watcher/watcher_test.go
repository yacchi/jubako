package watcher_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yacchi/jubako/watcher"
)

// testParams creates WatcherInitializerParams for testing with default config.
func testParams() watcher.WatcherInitializerParams {
	return testParamsWithConfig(watcher.NewWatchConfig())
}

// testParamsWithConfig creates WatcherInitializerParams with a custom config.
func testParamsWithConfig(cfg watcher.WatchConfig) watcher.WatcherInitializerParams {
	var mu sync.Mutex
	return watcher.WatcherInitializerParams{
		Fetch: func(ctx context.Context) (bool, []byte, error) {
			return true, nil, nil
		},
		OpMu:   &mu,
		Config: cfg,
	}
}

func TestDefaultCompareFunc(t *testing.T) {
	tests := []struct {
		name     string
		old      []byte
		new      []byte
		expected bool
	}{
		{"different", []byte("old"), []byte("new"), true},
		{"same", []byte("same"), []byte("same"), false},
		{"empty both", []byte{}, []byte{}, false},
		{"empty old", []byte{}, []byte("new"), true},
		{"empty new", []byte("old"), []byte{}, true},
		{"nil old", nil, []byte("new"), true},
		{"nil new", []byte("old"), nil, true},
		{"nil both", nil, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := watcher.DefaultCompareFunc(tt.old, tt.new)
			if result != tt.expected {
				t.Errorf("DefaultCompareFunc(%q, %q) = %v, want %v", tt.old, tt.new, result, tt.expected)
			}
		})
	}
}

func TestHashCompareFunc(t *testing.T) {
	tests := []struct {
		name     string
		old      []byte
		new      []byte
		expected bool
	}{
		{"different", []byte("old data"), []byte("new data"), true},
		{"same", []byte("same data"), []byte("same data"), false},
		{"empty both", []byte{}, []byte{}, false},
		{"empty old", []byte{}, []byte("new"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := watcher.HashCompareFunc(tt.old, tt.new)
			if result != tt.expected {
				t.Errorf("HashCompareFunc(%q, %q) = %v, want %v", tt.old, tt.new, result, tt.expected)
			}
		})
	}
}

func TestNewWatchConfig(t *testing.T) {
	// Default config
	cfg := watcher.NewWatchConfig()
	if cfg.PollInterval != watcher.DefaultPollInterval {
		t.Errorf("expected default PollInterval %v, got %v", watcher.DefaultPollInterval, cfg.PollInterval)
	}
	if cfg.CompareFunc == nil {
		t.Error("expected default CompareFunc, got nil")
	}

	// With custom options
	customInterval := 5 * time.Second
	cfg = watcher.NewWatchConfig(
		watcher.WithPollInterval(customInterval),
		watcher.WithCompareFunc(watcher.HashCompareFunc),
	)
	if cfg.PollInterval != customInterval {
		t.Errorf("expected PollInterval %v, got %v", customInterval, cfg.PollInterval)
	}
}

func TestWatchConfig_ApplyOptions(t *testing.T) {
	cfg := watcher.NewWatchConfig()
	originalInterval := cfg.PollInterval

	newInterval := 10 * time.Second
	cfg.ApplyOptions(watcher.WithPollInterval(newInterval))

	if cfg.PollInterval == originalInterval {
		t.Error("ApplyOptions did not modify PollInterval")
	}
	if cfg.PollInterval != newInterval {
		t.Errorf("expected PollInterval %v, got %v", newInterval, cfg.PollInterval)
	}
}

func TestWatchConfig_ApplyDefaults(t *testing.T) {
	t.Run("fills zero PollInterval", func(t *testing.T) {
		cfg := watcher.WatchConfig{PollInterval: 0}
		cfg.ApplyDefaults()
		if cfg.PollInterval != watcher.DefaultPollInterval {
			t.Errorf("expected PollInterval %v, got %v", watcher.DefaultPollInterval, cfg.PollInterval)
		}
	})

	t.Run("fills negative PollInterval", func(t *testing.T) {
		cfg := watcher.WatchConfig{PollInterval: -1 * time.Second}
		cfg.ApplyDefaults()
		if cfg.PollInterval != watcher.DefaultPollInterval {
			t.Errorf("expected PollInterval %v, got %v", watcher.DefaultPollInterval, cfg.PollInterval)
		}
	})

	t.Run("fills nil CompareFunc", func(t *testing.T) {
		cfg := watcher.WatchConfig{CompareFunc: nil}
		cfg.ApplyDefaults()
		if cfg.CompareFunc == nil {
			t.Error("expected CompareFunc to be set")
		}
	})

	t.Run("preserves non-zero PollInterval", func(t *testing.T) {
		customInterval := 5 * time.Second
		cfg := watcher.WatchConfig{PollInterval: customInterval}
		cfg.ApplyDefaults()
		if cfg.PollInterval != customInterval {
			t.Errorf("expected PollInterval %v, got %v", customInterval, cfg.PollInterval)
		}
	})

	t.Run("preserves non-nil CompareFunc", func(t *testing.T) {
		customCompare := func(old, new []byte) bool { return true }
		cfg := watcher.WatchConfig{CompareFunc: customCompare}
		cfg.ApplyDefaults()
		// Verify it's still the custom function (returns true for same data)
		if !cfg.CompareFunc([]byte("same"), []byte("same")) {
			t.Error("CompareFunc was replaced")
		}
	})

	t.Run("fills all zero/nil values at once", func(t *testing.T) {
		cfg := watcher.WatchConfig{} // All zero/nil
		cfg.ApplyDefaults()
		if cfg.PollInterval != watcher.DefaultPollInterval {
			t.Errorf("expected PollInterval %v, got %v", watcher.DefaultPollInterval, cfg.PollInterval)
		}
		if cfg.CompareFunc == nil {
			t.Error("expected CompareFunc to be set")
		}
	})
}

// mockPollOnce provides a FetchFunc for testing.
type mockPollOnce struct {
	data   []byte
	err    error
	calls  atomic.Int32
	dataCh chan []byte
	errCh  chan error
	mu     sync.Mutex
}

func newMockPollOnce(data []byte) *mockPollOnce {
	return &mockPollOnce{
		data:   data,
		dataCh: make(chan []byte, 10),
		errCh:  make(chan error, 10),
	}
}

func (m *mockPollOnce) Poll(ctx context.Context) (bool, []byte, error) {
	m.calls.Add(1)

	select {
	case data := <-m.dataCh:
		m.mu.Lock()
		m.data = data
		m.mu.Unlock()
		return true, data, nil
	case err := <-m.errCh:
		return false, nil, err
	default:
		m.mu.Lock()
		data := m.data
		m.mu.Unlock()
		return true, data, m.err
	}
}

func (m *mockPollOnce) Update(data []byte) {
	m.dataCh <- data
}

func (m *mockPollOnce) SetError(err error) {
	m.errCh <- err
}

func TestPollingWatcher_Basic(t *testing.T) {
	mock := newMockPollOnce([]byte("initial"))
	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(50 * time.Millisecond))
	w, err := watcher.NewPolling(mock.Poll)(testParamsWithConfig(cfg))
	if err != nil {
		t.Fatalf("NewPolling() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer w.Stop(context.Background())

	// Should receive initial data
	select {
	case result := <-w.Results():
		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		if string(result.Data) != "initial" {
			t.Errorf("expected initial, got %s", string(result.Data))
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for initial result")
	}

	// Update and wait for change detection
	mock.Update([]byte("updated"))

	select {
	case result := <-w.Results():
		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		if string(result.Data) != "updated" {
			t.Errorf("expected updated, got %s", string(result.Data))
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for updated result")
	}
}

func TestPollingWatcher_Error(t *testing.T) {
	mock := newMockPollOnce([]byte("data"))
	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(50 * time.Millisecond))
	w, err := watcher.NewPolling(mock.Poll)(testParamsWithConfig(cfg))
	if err != nil {
		t.Fatalf("NewPolling() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer w.Stop(context.Background())

	// Receive initial
	<-w.Results()

	// Send error
	testErr := errors.New("test error")
	mock.SetError(testErr)

	select {
	case result := <-w.Results():
		if result.Error == nil {
			t.Error("expected error, got nil")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for error result")
	}
}

func TestPollingWatcher_Stop(t *testing.T) {
	mock := newMockPollOnce([]byte("data"))
	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(50 * time.Millisecond))
	w, err := watcher.NewPolling(mock.Poll)(testParamsWithConfig(cfg))
	if err != nil {
		t.Fatalf("NewPolling() error: %v", err)
	}

	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Stop
	if err := w.Stop(ctx); err != nil {
		t.Errorf("Stop() error: %v", err)
	}

	// Results channel should be closed
	select {
	case _, ok := <-w.Results():
		if ok {
			// May receive pending result, try again
			select {
			case _, ok := <-w.Results():
				if ok {
					t.Error("expected channel to be closed")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("timeout waiting for channel close")
			}
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for channel close")
	}

	// Double stop should not error
	if err := w.Stop(ctx); err != nil {
		t.Errorf("second Stop() error: %v", err)
	}
}

func TestPollingWatcher_DoubleStart(t *testing.T) {
	mock := newMockPollOnce([]byte("data"))
	w, err := watcher.NewPolling(mock.Poll)(testParams())
	if err != nil {
		t.Fatalf("NewPolling() error: %v", err)
	}

	ctx := context.Background()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("first Start() error: %v", err)
	}
	defer w.Stop(ctx)

	// Second start should be no-op
	if err := w.Start(ctx); err != nil {
		t.Errorf("second Start() error: %v", err)
	}
}

func TestPollingWatcher_EventOnly(t *testing.T) {
	// Event-only poll function: returns changed=true, data=nil
	var pollCalls atomic.Int32
	eventOnlyPoll := func(ctx context.Context) (bool, []byte, error) {
		pollCalls.Add(1)
		return true, nil, nil // event-only: changed=true, data=nil
	}

	// Fetcher that provides actual data
	var fetchCalls atomic.Int32
	expectedData := []byte("fetched data")
	fetcher := func(ctx context.Context) (bool, []byte, error) {
		fetchCalls.Add(1)
		return true, expectedData, nil
	}

	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(50 * time.Millisecond))
	var mu sync.Mutex
	params := watcher.WatcherInitializerParams{
		Fetch:  fetcher,
		OpMu:   &mu,
		Config: cfg,
	}

	w, err := watcher.NewPolling(eventOnlyPoll)(params)
	if err != nil {
		t.Fatalf("NewPolling() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer w.Stop(context.Background())

	// Should receive data fetched via FetchFunc
	select {
	case result := <-w.Results():
		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		if string(result.Data) != string(expectedData) {
			t.Errorf("expected %q, got %q", expectedData, result.Data)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for result")
	}

	// Verify poll was called
	if pollCalls.Load() == 0 {
		t.Error("poll function was not called")
	}

	// Verify fetcher was called (for event-only notification)
	if fetchCalls.Load() == 0 {
		t.Error("fetcher was not called for event-only notification")
	}
}

func TestPollingWatcher_EventOnly_FetcherNoChange(t *testing.T) {
	// Event-only poll function: returns changed=true, data=nil
	eventOnlyPoll := func(ctx context.Context) (bool, []byte, error) {
		return true, nil, nil // event-only
	}

	// Fetcher that returns no change
	fetcher := func(ctx context.Context) (bool, []byte, error) {
		return false, nil, nil // no change
	}

	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(50 * time.Millisecond))
	var mu sync.Mutex
	params := watcher.WatcherInitializerParams{
		Fetch:  fetcher,
		OpMu:   &mu,
		Config: cfg,
	}

	w, err := watcher.NewPolling(eventOnlyPoll)(params)
	if err != nil {
		t.Fatalf("NewPolling() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer w.Stop(context.Background())

	// Should NOT receive any result because fetcher returns changed=false
	select {
	case result := <-w.Results():
		t.Errorf("unexpected result: %+v", result)
	case <-ctx.Done():
		// Expected - no results because fetcher says no change
	}
}

func TestPollingWatcher_EventOnly_FetcherError(t *testing.T) {
	// Event-only poll function
	eventOnlyPoll := func(ctx context.Context) (bool, []byte, error) {
		return true, nil, nil
	}

	// Fetcher that returns an error
	fetchErr := errors.New("fetch error")
	fetcher := func(ctx context.Context) (bool, []byte, error) {
		return false, nil, fetchErr
	}

	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(50 * time.Millisecond))
	var mu sync.Mutex
	params := watcher.WatcherInitializerParams{
		Fetch:  fetcher,
		OpMu:   &mu,
		Config: cfg,
	}

	w, err := watcher.NewPolling(eventOnlyPoll)(params)
	if err != nil {
		t.Fatalf("NewPolling() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer w.Stop(context.Background())

	// Should receive error from fetcher
	select {
	case result := <-w.Results():
		if result.Error == nil {
			t.Error("expected error, got nil")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for error result")
	}
}

// Mock SubscriptionHandler for testing
type mockSubscriptionHandler struct {
	notifyFn watcher.NotifyFunc
	stopErr  error
}

func (h *mockSubscriptionHandler) Subscribe(ctx context.Context, notify watcher.NotifyFunc) (watcher.StopFunc, error) {
	h.notifyFn = notify
	return func(ctx context.Context) error {
		h.notifyFn = nil
		return h.stopErr
	}, nil
}

func (h *mockSubscriptionHandler) Notify(data []byte, err error) {
	if h.notifyFn != nil {
		h.notifyFn(data, err)
	}
}

func TestSubscriptionWatcher_Basic(t *testing.T) {
	handler := &mockSubscriptionHandler{}
	w, err := watcher.NewSubscription(handler)(testParams())
	if err != nil {
		t.Fatalf("NewSubscription() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer w.Stop(context.Background())

	// Send notification in goroutine to avoid blocking
	go func() {
		time.Sleep(10 * time.Millisecond)
		handler.Notify([]byte("data"), nil)
	}()

	select {
	case result := <-w.Results():
		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		if string(result.Data) != "data" {
			t.Errorf("expected data, got %s", string(result.Data))
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for result")
	}
}

func TestSubscriptionWatcher_Error(t *testing.T) {
	handler := &mockSubscriptionHandler{}
	w, err := watcher.NewSubscription(handler)(testParams())
	if err != nil {
		t.Fatalf("NewSubscription() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer w.Stop(context.Background())

	// Send error notification in goroutine
	go func() {
		time.Sleep(10 * time.Millisecond)
		testErr := errors.New("test error")
		handler.Notify(nil, testErr)
	}()

	select {
	case result := <-w.Results():
		if result.Error == nil {
			t.Error("expected error, got nil")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for error result")
	}
}

func TestSubscriptionWatcher_Stop(t *testing.T) {
	handler := &mockSubscriptionHandler{}
	w, err := watcher.NewSubscription(handler)(testParams())
	if err != nil {
		t.Fatalf("NewSubscription() error: %v", err)
	}

	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if err := w.Stop(ctx); err != nil {
		t.Errorf("Stop() error: %v", err)
	}

	// Double stop should not error
	if err := w.Stop(ctx); err != nil {
		t.Errorf("second Stop() error: %v", err)
	}
}

func TestSubscriptionWatcher_DoubleStart(t *testing.T) {
	handler := &mockSubscriptionHandler{}
	w, err := watcher.NewSubscription(handler)(testParams())
	if err != nil {
		t.Fatalf("NewSubscription() error: %v", err)
	}

	ctx := context.Background()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("first Start() error: %v", err)
	}
	defer w.Stop(ctx)

	// Second start should be no-op
	if err := w.Start(ctx); err != nil {
		t.Errorf("second Start() error: %v", err)
	}
}

// failingSubscriptionHandler returns an error on Subscribe
type failingSubscriptionHandler struct{}

func (h *failingSubscriptionHandler) Subscribe(ctx context.Context, notify watcher.NotifyFunc) (watcher.StopFunc, error) {
	return nil, errors.New("subscribe failed")
}

func TestSubscriptionWatcher_SubscribeError(t *testing.T) {
	handler := &failingSubscriptionHandler{}
	w, err := watcher.NewSubscription(handler)(testParams())
	if err != nil {
		t.Fatalf("NewSubscription() error: %v", err)
	}

	ctx := context.Background()

	err = w.Start(ctx)
	if err == nil {
		t.Error("expected error from Start, got nil")
		w.Stop(ctx)
	}
}

func TestNoopWatcher_Basic(t *testing.T) {
	w, err := watcher.NewNoop()(testParams())
	if err != nil {
		t.Fatalf("NewNoop() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// NoopWatcher should not emit any results
	select {
	case result := <-w.Results():
		t.Errorf("unexpected result: %+v", result)
	case <-ctx.Done():
		// Expected - no results
	}

	if err := w.Stop(context.Background()); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

func TestNoopWatcher_Stop(t *testing.T) {
	w, err := watcher.NewNoop()(testParams())
	if err != nil {
		t.Fatalf("NewNoop() error: %v", err)
	}

	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if err := w.Stop(ctx); err != nil {
		t.Errorf("Stop() error: %v", err)
	}

	// Results channel should be closed
	select {
	case _, ok := <-w.Results():
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for channel close")
	}

	// Double stop should not error
	if err := w.Stop(ctx); err != nil {
		t.Errorf("second Stop() error: %v", err)
	}
}

func TestNoopWatcher_DoubleStart(t *testing.T) {
	w, err := watcher.NewNoop()(testParams())
	if err != nil {
		t.Fatalf("NewNoop() error: %v", err)
	}

	ctx := context.Background()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("first Start() error: %v", err)
	}
	defer w.Stop(ctx)

	// Second start should be no-op
	if err := w.Start(ctx); err != nil {
		t.Errorf("second Start() error: %v", err)
	}
}

func TestFetchFunc(t *testing.T) {
	called := false
	expectedData := []byte("test data")

	fetch := watcher.FetchFunc(func(ctx context.Context) (bool, []byte, error) {
		called = true
		return true, expectedData, nil
	})

	changed, data, err := fetch(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("function was not called")
	}
	if !changed {
		t.Error("expected changed=true")
	}
	if string(data) != string(expectedData) {
		t.Errorf("expected %s, got %s", expectedData, data)
	}
}

func TestSubscriptionHandlerFunc(t *testing.T) {
	var capturedNotify watcher.NotifyFunc
	stopCalled := false

	fn := watcher.SubscriptionHandlerFunc(func(ctx context.Context, notify watcher.NotifyFunc) (watcher.StopFunc, error) {
		capturedNotify = notify
		return func(ctx context.Context) error {
			stopCalled = true
			return nil
		}, nil
	})

	stop, err := fn.Subscribe(context.Background(), func(data []byte, err error) {})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if capturedNotify == nil {
		t.Error("notify function was not captured")
	}

	if err := stop(context.Background()); err != nil {
		t.Errorf("stop error: %v", err)
	}
	if !stopCalled {
		t.Error("stop was not called")
	}
}
