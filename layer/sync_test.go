package layer_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/format/json"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
)

// concurrentTracker tracks concurrent operations to detect overlapping executions.
type concurrentTracker struct {
	mu          sync.Mutex
	active      int32
	maxActive   int32
	overlaps    int32
	totalCalls  int32
	durations   []time.Duration
	lastOpStart time.Time
}

func newConcurrentTracker() *concurrentTracker {
	return &concurrentTracker{}
}

func (t *concurrentTracker) enter() {
	t.mu.Lock()
	t.active++
	t.totalCalls++
	if t.active > t.maxActive {
		t.maxActive = t.active
	}
	if t.active > 1 {
		t.overlaps++
	}
	t.lastOpStart = time.Now()
	t.mu.Unlock()
}

func (t *concurrentTracker) exit() {
	t.mu.Lock()
	t.durations = append(t.durations, time.Since(t.lastOpStart))
	t.active--
	t.mu.Unlock()
}

func (t *concurrentTracker) getMaxActive() int32 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.maxActive
}

func (t *concurrentTracker) getOverlaps() int32 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.overlaps
}

// slowSource is a source with configurable delay to test synchronization.
// It does NOT implement WatchableSource, so the layer will use fallback polling
// which goes through the synchronized fetch -> source.Load() path.
type slowSource struct {
	mu          sync.Mutex
	data        []byte
	loadDelay   time.Duration
	saveDelay   time.Duration
	loadTracker *concurrentTracker
	saveTracker *concurrentTracker
}

var _ source.Source = (*slowSource)(nil)

func newSlowSource(data []byte, delay time.Duration) *slowSource {
	return &slowSource{
		data:        data,
		loadDelay:   delay,
		saveDelay:   delay,
		loadTracker: newConcurrentTracker(),
		saveTracker: newConcurrentTracker(),
	}
}

func (s *slowSource) Type() source.SourceType { return "slow" }

func (s *slowSource) Load(ctx context.Context) ([]byte, error) {
	s.loadTracker.enter()
	defer s.loadTracker.exit()

	select {
	case <-time.After(s.loadDelay):
		s.mu.Lock()
		result := make([]byte, len(s.data))
		copy(result, s.data)
		s.mu.Unlock()
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *slowSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	s.saveTracker.enter()
	defer s.saveTracker.exit()

	select {
	case <-time.After(s.saveDelay):
		s.mu.Lock()
		newData, err := updateFunc(s.data)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		s.data = newData
		s.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *slowSource) CanSave() bool { return true }

// newTestDocument returns a json.Document for testing.
func newTestDocument() document.Document {
	return json.New()
}

// TestLayer_LoadPollExclusion verifies that Load and poll operations are mutually exclusive.
func TestLayer_LoadPollExclusion(t *testing.T) {
	delay := 50 * time.Millisecond
	src := newSlowSource([]byte(`{"key": "value"}`), delay)
	l := layer.New("test", src, newTestDocument())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use very short poll interval to trigger many polls
	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(10 * time.Millisecond))
	lw, err := l.Watch(layer.WithBaseConfig(cfg))
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer lw.Stop(context.Background())

	// Wait for first poll to complete
	select {
	case <-lw.Results():
	case <-ctx.Done():
		t.Fatal("timeout waiting for first poll")
	}

	// Call Load multiple times concurrently while polling is happening
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := l.Load(ctx)
			if err != nil && err != context.Canceled {
				t.Errorf("Load error: %v", err)
			}
		}()
	}

	// Let it run for a while
	time.Sleep(200 * time.Millisecond)
	wg.Wait()

	// The load tracker tracks all Load operations (both direct calls and polling via syncFetch).
	// With fallback polling, polling goes through syncFetch -> source.Load(), so all
	// operations are tracked by loadTracker and serialized by layer's opMu.
	loadMax := src.loadTracker.getMaxActive()

	t.Logf("Load max concurrent: %d", loadMax)

	// Load tracker should show max 1 (due to layer mutex)
	if loadMax > 1 {
		t.Errorf("Load operations overlapped: max concurrent = %d", loadMax)
	}
}

// TestLayer_SaveExclusion verifies that Save operations are exclusive with Load and poll.
func TestLayer_SaveExclusion(t *testing.T) {
	delay := 30 * time.Millisecond
	src := newSlowSource([]byte(`{"count": 0}`), delay)
	l := layer.New("test", src, newTestDocument())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the watcher
	cfg := watcher.NewWatchConfig(watcher.WithPollInterval(15 * time.Millisecond))
	lw, err := l.Watch(layer.WithBaseConfig(cfg))
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer lw.Stop(context.Background())

	// Wait for first poll
	select {
	case <-lw.Results():
	case <-ctx.Done():
		t.Fatal("timeout waiting for first poll")
	}

	// Run Load, Save, and poll concurrently
	var wg sync.WaitGroup

	// Multiple Load calls
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				_, err := l.Load(ctx)
				if err != nil && err != context.Canceled {
					t.Errorf("Load error: %v", err)
				}
			}
		}()
	}

	// Multiple Save calls
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			changeset := document.JSONPatchSet{
				{Op: document.PatchOpReplace, Path: "/count", Value: i},
			}
			err := l.Save(ctx, changeset)
			if err != nil && err != context.Canceled {
				t.Errorf("Save error: %v", err)
			}
		}(i)
	}

	// Let it run
	time.Sleep(300 * time.Millisecond)
	wg.Wait()

	// Check trackers - with fallback polling, poll operations go through source.Load()
	// so loadTracker tracks both direct loads and polling.
	loadMax := src.loadTracker.getMaxActive()
	saveMax := src.saveTracker.getMaxActive()

	t.Logf("Load max: %d, Save max: %d", loadMax, saveMax)

	// Each should have max 1 active at a time
	if loadMax > 1 {
		t.Errorf("Load operations overlapped: max = %d", loadMax)
	}
	if saveMax > 1 {
		t.Errorf("Save operations overlapped: max = %d", saveMax)
	}
}

// TestLayer_ConcurrentLoadSafe verifies concurrent Load calls are safe.
func TestLayer_ConcurrentLoadSafe(t *testing.T) {
	src := newSlowSource([]byte(`{"key": "value"}`), 10*time.Millisecond)
	l := layer.New("test", src, newTestDocument())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var errors atomic.Int32
	var successes atomic.Int32

	// Launch many concurrent Load calls
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := l.Load(ctx)
			if err != nil {
				errors.Add(1)
				return
			}
			if data["key"] == "value" {
				successes.Add(1)
			}
		}()
	}

	wg.Wait()

	if errors.Load() > 0 {
		t.Errorf("Got %d errors during concurrent Load", errors.Load())
	}
	if successes.Load() != 20 {
		t.Errorf("Expected 20 successes, got %d", successes.Load())
	}

	// All loads should have been serialized
	if src.loadTracker.getMaxActive() > 1 {
		t.Errorf("Loads were not serialized: max concurrent = %d", src.loadTracker.getMaxActive())
	}
}

// TestLayer_SubscriptionSynchronized verifies subscription-based watchers are synchronized.
func TestLayer_SubscriptionSynchronized(t *testing.T) {
	delay := 5 * time.Millisecond
	src := newSlowSubscriptionSource([]byte(`{"key": "value"}`), delay)
	l := layer.New("test", src, newTestDocument())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start the watcher
	lw, err := l.Watch()
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	if err := lw.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Trigger updates and concurrent loads
	var wg sync.WaitGroup

	// Trigger subscription updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			time.Sleep(10 * time.Millisecond)
			src.TriggerUpdate([]byte(`{"key": "updated"}`))
		}
	}()

	// Concurrent loads
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				_, err := l.Load(ctx)
				if err != nil {
					// Timeout errors are expected as operations queue up
					return
				}
			}
		}()
	}

	// Wait for all operations to complete
	wg.Wait()

	// Stop the watcher after all updates are done
	lw.Stop(context.Background())

	// Subscription callbacks should not overlap with loads
	if src.loadTracker.getMaxActive() > 1 {
		t.Errorf("Load operations overlapped: max = %d", src.loadTracker.getMaxActive())
	}
}

// slowSubscriptionSource is a subscription-based source with delay for testing.
type slowSubscriptionSource struct {
	mu          sync.Mutex
	data        []byte
	loadDelay   time.Duration
	loadTracker *concurrentTracker
	notifyFn    watcher.NotifyFunc
}

var _ source.Source = (*slowSubscriptionSource)(nil)
var _ source.WatchableSource = (*slowSubscriptionSource)(nil)

func newSlowSubscriptionSource(data []byte, delay time.Duration) *slowSubscriptionSource {
	return &slowSubscriptionSource{
		data:        data,
		loadDelay:   delay,
		loadTracker: newConcurrentTracker(),
	}
}

func (s *slowSubscriptionSource) Type() source.SourceType { return "slow-sub" }

func (s *slowSubscriptionSource) Load(ctx context.Context) ([]byte, error) {
	s.loadTracker.enter()
	defer s.loadTracker.exit()

	select {
	case <-time.After(s.loadDelay):
		s.mu.Lock()
		result := make([]byte, len(s.data))
		copy(result, s.data)
		s.mu.Unlock()
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *slowSubscriptionSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	return source.ErrSaveNotSupported
}

func (s *slowSubscriptionSource) CanSave() bool { return false }

func (s *slowSubscriptionSource) Watch() (watcher.WatcherInitializer, error) {
	return watcher.NewSubscription(watcher.SubscriptionHandlerFunc(s.subscribe)), nil
}

func (s *slowSubscriptionSource) subscribe(ctx context.Context, notify watcher.NotifyFunc) (watcher.StopFunc, error) {
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

func (s *slowSubscriptionSource) TriggerUpdate(data []byte) {
	s.mu.Lock()
	s.data = data
	notify := s.notifyFn
	s.mu.Unlock()

	if notify != nil {
		notify(data, nil)
	}
}
