package watcher_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yacchi/jubako/watcher"
)

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

// Mock PollHandler for testing
type mockPollHandler struct {
	data    []byte
	err     error
	calls   atomic.Int32
	dataCh  chan []byte
	errCh   chan error
	changed bool
}

func newMockPollHandler(data []byte) *mockPollHandler {
	return &mockPollHandler{
		data:   data,
		dataCh: make(chan []byte, 10),
		errCh:  make(chan error, 10),
	}
}

func (h *mockPollHandler) Poll(ctx context.Context) ([]byte, error) {
	h.calls.Add(1)

	select {
	case data := <-h.dataCh:
		h.data = data
		return data, nil
	case err := <-h.errCh:
		return nil, err
	default:
		return h.data, h.err
	}
}

func (h *mockPollHandler) Update(data []byte) {
	h.dataCh <- data
}

func (h *mockPollHandler) SetError(err error) {
	h.errCh <- err
}

func TestPollingWatcher_Basic(t *testing.T) {
	handler := newMockPollHandler([]byte("initial"))
	w := watcher.NewPolling(handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(50 * time.Millisecond))
	if err := w.Start(ctx, cfg); err != nil {
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
	handler.Update([]byte("updated"))

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
	handler := newMockPollHandler([]byte("data"))
	w := watcher.NewPolling(handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(50 * time.Millisecond))
	if err := w.Start(ctx, cfg); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer w.Stop(context.Background())

	// Receive initial
	<-w.Results()

	// Send error
	testErr := errors.New("test error")
	handler.SetError(testErr)

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
	handler := newMockPollHandler([]byte("data"))
	w := watcher.NewPolling(handler)

	ctx := context.Background()
	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(50 * time.Millisecond))
	if err := w.Start(ctx, cfg); err != nil {
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
	handler := newMockPollHandler([]byte("data"))
	w := watcher.NewPolling(handler)

	ctx := context.Background()
	cfg := watcher.NewWatchConfig()

	if err := w.Start(ctx, cfg); err != nil {
		t.Fatalf("first Start() error: %v", err)
	}
	defer w.Stop(ctx)

	// Second start should be no-op
	if err := w.Start(ctx, cfg); err != nil {
		t.Errorf("second Start() error: %v", err)
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
	w := watcher.NewSubscription(handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := watcher.NewWatchConfig()
	if err := w.Start(ctx, cfg); err != nil {
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
	w := watcher.NewSubscription(handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := watcher.NewWatchConfig()
	if err := w.Start(ctx, cfg); err != nil {
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
	w := watcher.NewSubscription(handler)

	ctx := context.Background()
	cfg := watcher.NewWatchConfig()
	if err := w.Start(ctx, cfg); err != nil {
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
	w := watcher.NewSubscription(handler)

	ctx := context.Background()
	cfg := watcher.NewWatchConfig()

	if err := w.Start(ctx, cfg); err != nil {
		t.Fatalf("first Start() error: %v", err)
	}
	defer w.Stop(ctx)

	// Second start should be no-op
	if err := w.Start(ctx, cfg); err != nil {
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
	w := watcher.NewSubscription(handler)

	ctx := context.Background()
	cfg := watcher.NewWatchConfig()

	err := w.Start(ctx, cfg)
	if err == nil {
		t.Error("expected error from Start, got nil")
		w.Stop(ctx)
	}
}

func TestNoopWatcher_Basic(t *testing.T) {
	w := watcher.NewNoop()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cfg := watcher.NewWatchConfig()
	if err := w.Start(ctx, cfg); err != nil {
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
	w := watcher.NewNoop()

	ctx := context.Background()
	cfg := watcher.NewWatchConfig()
	if err := w.Start(ctx, cfg); err != nil {
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
	w := watcher.NewNoop()

	ctx := context.Background()
	cfg := watcher.NewWatchConfig()

	if err := w.Start(ctx, cfg); err != nil {
		t.Fatalf("first Start() error: %v", err)
	}
	defer w.Stop(ctx)

	// Second start should be no-op
	if err := w.Start(ctx, cfg); err != nil {
		t.Errorf("second Start() error: %v", err)
	}
}

func TestPollHandlerFunc(t *testing.T) {
	called := false
	expectedData := []byte("test data")

	fn := watcher.PollHandlerFunc(func(ctx context.Context) ([]byte, error) {
		called = true
		return expectedData, nil
	})

	data, err := fn.Poll(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("function was not called")
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
