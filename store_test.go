package jubako

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/layer/mapdata"
)

type testConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func TestNew(t *testing.T) {
	t.Run("creates store with zero value", func(t *testing.T) {
		store := New[testConfig]()
		if store == nil {
			t.Fatal("New() returned nil")
		}

		cfg := store.Get()
		if cfg.Host != "" || cfg.Port != 0 {
			t.Errorf("Get() = %+v, want zero value", cfg)
		}
	})

	t.Run("int config", func(t *testing.T) {
		store := New[int]()
		if got := store.Get(); got != 0 {
			t.Errorf("Get() = %v, want 0", got)
		}
	})

	t.Run("string config", func(t *testing.T) {
		store := New[string]()
		if got := store.Get(); got != "" {
			t.Errorf("Get() = %q, want empty string", got)
		}
	})

	t.Run("with priority step option", func(t *testing.T) {
		store := New[testConfig](WithPriorityStep(100))

		// Add layers without explicit priority
		err := store.Add(mapdata.New("first", map[string]any{"host": "first"}))
		if err != nil {
			t.Fatalf("Add(first) error = %v", err)
		}

		err = store.Add(mapdata.New("second", map[string]any{"host": "second"}))
		if err != nil {
			t.Fatalf("Add(second) error = %v", err)
		}

		// Insert between with explicit priority 50
		err = store.Add(mapdata.New("middle", map[string]any{"host": "middle"}), WithPriority(50))
		if err != nil {
			t.Fatalf("Add(middle) error = %v", err)
		}

		// first=0, middle=50, second=100
		layers := store.ListLayers()
		expectedOrder := []layer.Name{"first", "middle", "second"}
		for i, info := range layers {
			if info.Name() != expectedOrder[i] {
				t.Errorf("layers[%d].Name = %q, want %q", i, info.Name(), expectedOrder[i])
			}
		}
	})
}

func TestStore_Add(t *testing.T) {
	t.Run("add single layer", func(t *testing.T) {
		store := New[testConfig]()

		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}), WithPriority(PriorityUser))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	})

	t.Run("add multiple layers", func(t *testing.T) {
		store := New[testConfig]()

		layers := []struct {
			name     layer.Name
			priority layer.Priority
		}{
			{"defaults", PriorityDefaults},
			{"user", PriorityUser},
			{"project", PriorityProject},
			{"env", PriorityEnv},
			{"flags", PriorityFlags},
		}

		for _, l := range layers {
			err := store.Add(mapdata.New(l.name, map[string]any{"host": "localhost", "port": 8080}), WithPriority(l.priority))
			if err != nil {
				t.Fatalf("Add(%q) error = %v", l.name, err)
			}
		}
	})

	t.Run("duplicate layer name", func(t *testing.T) {
		store := New[testConfig]()

		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}), WithPriority(PriorityUser))
		if err != nil {
			t.Fatalf("First Add() error = %v", err)
		}

		err = store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}), WithPriority(PriorityProject))
		if err == nil {
			t.Error("Add() with duplicate name should return error")
		}
	})

	t.Run("same priority different names", func(t *testing.T) {
		store := New[testConfig]()

		err := store.Add(mapdata.New("layer1", map[string]any{"host": "localhost"}), WithPriority(PriorityUser))
		if err != nil {
			t.Fatalf("Add(layer1) error = %v", err)
		}

		err = store.Add(mapdata.New("layer2", map[string]any{"port": 8080}), WithPriority(PriorityUser))
		if err != nil {
			t.Fatalf("Add(layer2) error = %v", err)
		}
	})

	t.Run("add without priority uses insertion order", func(t *testing.T) {
		store := New[testConfig]()
		ctx := context.Background()

		// Add layers without explicit priority - later layers should override earlier ones
		err := store.Add(mapdata.New("base", map[string]any{"host": "base", "port": 8080}))
		if err != nil {
			t.Fatalf("Add(base) error = %v", err)
		}

		err = store.Add(mapdata.New("override", map[string]any{"host": "override"}))
		if err != nil {
			t.Fatalf("Add(override) error = %v", err)
		}

		err = store.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		cfg := store.Get()
		// override layer should win for host
		if cfg.Host != "override" {
			t.Errorf("Host = %q, want %q (from override layer)", cfg.Host, "override")
		}
		// port should come from base (not overridden)
		if cfg.Port != 8080 {
			t.Errorf("Port = %d, want %d (from base layer)", cfg.Port, 8080)
		}
	})

	t.Run("add without priority preserves insertion order", func(t *testing.T) {
		store := New[testConfig]()

		// Add multiple layers without priority
		err := store.Add(mapdata.New("first", map[string]any{"host": "first"}))
		if err != nil {
			t.Fatalf("Add(first) error = %v", err)
		}

		err = store.Add(mapdata.New("second", map[string]any{"port": 8080}))
		if err != nil {
			t.Fatalf("Add(second) error = %v", err)
		}

		err = store.Add(mapdata.New("third", map[string]any{"host": "third"}))
		if err != nil {
			t.Fatalf("Add(third) error = %v", err)
		}

		// Verify layer order maintains insertion order
		layers := store.ListLayers()
		expectedOrder := []layer.Name{"first", "second", "third"}
		for i, info := range layers {
			if info.Name() != expectedOrder[i] {
				t.Errorf("layers[%d].Name = %q, want %q", i, info.Name(), expectedOrder[i])
			}
		}
	})

	t.Run("auto-assigned priority allows explicit priority to control order", func(t *testing.T) {
		store := New[testConfig]()

		// First layer gets auto-priority 0
		err := store.Add(mapdata.New("first", map[string]any{"host": "first"}))
		if err != nil {
			t.Fatalf("Add(first) error = %v", err)
		}

		// Second layer gets auto-priority 10
		err = store.Add(mapdata.New("second", map[string]any{"host": "second"}))
		if err != nil {
			t.Fatalf("Add(second) error = %v", err)
		}

		// Third layer with explicit priority 5 should be between first and second
		err = store.Add(mapdata.New("middle", map[string]any{"host": "middle"}), WithPriority(5))
		if err != nil {
			t.Fatalf("Add(middle) error = %v", err)
		}

		layers := store.ListLayers()
		expectedOrder := []layer.Name{"first", "middle", "second"}
		for i, info := range layers {
			if info.Name() != expectedOrder[i] {
				t.Errorf("layers[%d].Name = %q, want %q", i, info.Name(), expectedOrder[i])
			}
		}
	})
}

func TestStore_Get(t *testing.T) {
	store := New[testConfig]()

	// Get should return zero value initially
	cfg := store.Get()
	if cfg.Host != "" || cfg.Port != 0 {
		t.Errorf("Get() = %+v, want zero value", cfg)
	}

	// Multiple gets should return the same value
	for i := 0; i < 10; i++ {
		cfg := store.Get()
		if cfg.Host != "" || cfg.Port != 0 {
			t.Errorf("Get() #%d = %+v, want zero value", i, cfg)
		}
	}
}

func TestStore_Subscribe(t *testing.T) {
	t.Run("single subscriber", func(t *testing.T) {
		store := New[int]()
		var called int
		var lastValue int

		unsubscribe := store.Subscribe(func(v int) {
			called++
			lastValue = v
		})
		defer unsubscribe()

		// Manually trigger notification (since we don't have materialization yet)
		store.resolved.Set(42)
		store.notifySubscribers()

		if called != 1 {
			t.Errorf("subscriber called %d times, want 1", called)
		}
		if lastValue != 42 {
			t.Errorf("subscriber received %v, want 42", lastValue)
		}
	})

	t.Run("multiple subscribers", func(t *testing.T) {
		store := New[int]()
		var called1, called2, called3 int

		unsub1 := store.Subscribe(func(v int) { called1++ })
		unsub2 := store.Subscribe(func(v int) { called2++ })
		unsub3 := store.Subscribe(func(v int) { called3++ })
		defer unsub1()
		defer unsub2()
		defer unsub3()

		store.resolved.Set(42)
		store.notifySubscribers()

		if called1 != 1 || called2 != 1 || called3 != 1 {
			t.Errorf("subscribers called %d, %d, %d times, want 1, 1, 1", called1, called2, called3)
		}
	})

	t.Run("unsubscribe", func(t *testing.T) {
		store := New[int]()
		var called int

		unsubscribe := store.Subscribe(func(v int) { called++ })

		store.resolved.Set(1)
		store.notifySubscribers()
		if called != 1 {
			t.Errorf("subscriber called %d times, want 1", called)
		}

		unsubscribe()

		store.resolved.Set(2)
		store.notifySubscribers()
		if called != 1 {
			t.Errorf("after unsubscribe, subscriber called %d times, want 1", called)
		}
	})

	t.Run("subscriber order", func(t *testing.T) {
		store := New[int]()
		var order []int

		store.Subscribe(func(v int) { order = append(order, 1) })
		store.Subscribe(func(v int) { order = append(order, 2) })
		store.Subscribe(func(v int) { order = append(order, 3) })

		store.resolved.Set(42)
		store.notifySubscribers()

		if len(order) != 3 {
			t.Fatalf("len(order) = %d, want 3", len(order))
		}
		if order[0] != 1 || order[1] != 2 || order[2] != 3 {
			t.Errorf("subscriber order = %v, want [1, 2, 3]", order)
		}
	})

	t.Run("subscriber can call store methods during notification", func(t *testing.T) {
		ctx := context.Background()
		store := New[testConfig]()

		err := store.Add(mapdata.New("defaults", map[string]any{"host": "localhost", "port": 8080}), WithPriority(PriorityDefaults))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		done := make(chan struct{})
		var once sync.Once
		store.Subscribe(func(cfg testConfig) {
			// If notifications happen under store locks, this can deadlock.
			rv := store.GetAt("/host")
			if !rv.Exists {
				t.Error("GetAt(/host).Exists = false, want true")
			}
			if rv.Value != "localhost" {
				t.Errorf("GetAt(/host).Value = %v, want %q", rv.Value, "localhost")
			}
			once.Do(func() { close(done) })
		})

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("timeout waiting for subscriber callback; possible deadlock")
		}
	})
}

func TestStore_LayerOrder(t *testing.T) {
	store := New[testConfig]()

	// Add layers in random order
	layerDefs := []struct {
		name     layer.Name
		priority layer.Priority
	}{
		{"flags", PriorityFlags},
		{"defaults", PriorityDefaults},
		{"env", PriorityEnv},
		{"user", PriorityUser},
		{"project", PriorityProject},
	}

	for _, l := range layerDefs {
		err := store.Add(mapdata.New(l.name, map[string]any{"host": "localhost", "port": 8080}), WithPriority(l.priority))
		if err != nil {
			t.Fatalf("Add(%q) error = %v", l.name, err)
		}
	}

	layers := store.ListLayers()

	// Verify sorted order (lowest priority first)
	expectedOrder := []layer.Name{"defaults", "user", "project", "env", "flags"}
	for i, info := range layers {
		if info.Name() != expectedOrder[i] {
			t.Errorf("layers[%d].Name = %q, want %q", i, info.Name(), expectedOrder[i])
		}
	}

	// Verify priorities are in ascending order
	for i := 0; i < len(layers)-1; i++ {
		if layers[i].Priority() > layers[i+1].Priority() {
			t.Errorf("layers[%d].Priority (%d) > layers[%d].Priority (%d)",
				i, layers[i].Priority(), i+1, layers[i+1].Priority())
		}
	}
}

func TestStore_LayerOrder_StableSort(t *testing.T) {
	store := New[testConfig]()

	// Add layers with same priority
	err := store.Add(mapdata.New("layer1", map[string]any{"host": "localhost"}), WithPriority(PriorityUser))
	if err != nil {
		t.Fatalf("Add(layer1) error = %v", err)
	}

	err = store.Add(mapdata.New("layer2", map[string]any{"port": 8080}), WithPriority(PriorityUser))
	if err != nil {
		t.Fatalf("Add(layer2) error = %v", err)
	}

	err = store.Add(mapdata.New("layer3", map[string]any{"host": "example.com"}), WithPriority(PriorityUser))
	if err != nil {
		t.Fatalf("Add(layer3) error = %v", err)
	}

	layers := store.ListLayers()

	// With same priority, order should be preserved (stable sort)
	expectedOrder := []layer.Name{"layer1", "layer2", "layer3"}
	for i, info := range layers {
		if info.Name() != expectedOrder[i] {
			t.Errorf("layers[%d].Name = %q, want %q", i, info.Name(), expectedOrder[i])
		}
	}
}

func TestStore_ConcurrentAdd(t *testing.T) {
	store := New[testConfig]()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	errors := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			// Use unique names to avoid duplicate errors
			err := store.Add(mapdata.New(layer.Name(rune('A'+id)), map[string]any{"host": "localhost", "port": 8080}), WithPriority(PriorityUser))
			errors[id] = err
		}(i)
	}

	wg.Wait()

	// All additions should succeed (unique names)
	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d: Add() error = %v", i, err)
		}
	}
}

func TestStore_ConcurrentGet(t *testing.T) {
	store := New[testConfig]()
	const goroutines = 100
	const iterations = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = store.Get()
			}
		}()
	}

	wg.Wait()
}

func TestStore_ConcurrentSubscribe(t *testing.T) {
	store := New[int]()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			unsubscribe := store.Subscribe(func(v int) {
				// Do nothing
			})
			unsubscribe()
		}()
	}

	wg.Wait()
}

func TestStore_ConcurrentNotify(t *testing.T) {
	store := New[int]()
	const subscribers = 20
	const notifiers = 10
	const duration = 100 * time.Millisecond

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var totalCalls int64

	// Start subscribers
	wg.Add(subscribers)
	for i := 0; i < subscribers; i++ {
		go func() {
			defer wg.Done()
			unsubscribe := store.Subscribe(func(v int) {
				atomic.AddInt64(&totalCalls, 1)
			})
			defer unsubscribe()

			<-stop
		}()
	}

	// Give subscribers time to register
	time.Sleep(10 * time.Millisecond)

	// Start notifiers
	wg.Add(notifiers)
	for i := 0; i < notifiers; i++ {
		go func(id int) {
			defer wg.Done()
			counter := 0
			for {
				select {
				case <-stop:
					return
				default:
					store.resolved.Set(id*1000 + counter)
					store.notifySubscribers()
					counter++
				}
			}
		}(i)
	}

	// Run for duration
	time.Sleep(duration)
	close(stop)
	wg.Wait()

	// Verify that subscribers were called
	if atomic.LoadInt64(&totalCalls) == 0 {
		t.Error("subscribers were never called")
	}
}

func TestStore_ReferenceStability(t *testing.T) {
	// This test demonstrates the key feature of Store:
	// references remain valid even when config changes

	store := New[testConfig]()

	// Get a reference
	ref := store.resolved

	// Modify through store
	store.resolved.Set(testConfig{Host: "localhost", Port: 8080})

	// Reference should still be valid
	cfg := ref.Get()
	if cfg.Host != "localhost" || cfg.Port != 8080 {
		t.Errorf("ref.Get() = %+v, want {localhost 8080}", cfg)
	}
}

func BenchmarkStore_Get(b *testing.B) {
	store := New[testConfig]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.Get()
	}
}

func BenchmarkStore_Add(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store := New[testConfig]()
		_ = store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}), WithPriority(PriorityUser))
	}
}

func BenchmarkStore_ListLayers(b *testing.B) {
	store := New[testConfig]()

	// Add multiple layers
	layerDefs := []struct {
		name     layer.Name
		priority layer.Priority
	}{
		{"defaults", PriorityDefaults},
		{"user", PriorityUser},
		{"project", PriorityProject},
		{"env", PriorityEnv},
		{"flags", PriorityFlags},
	}

	for _, l := range layerDefs {
		_ = store.Add(mapdata.New(l.name, map[string]any{"host": "localhost", "port": 8080}), WithPriority(l.priority))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.ListLayers()
	}
}

func BenchmarkStore_Subscribe(b *testing.B) {
	store := New[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unsubscribe := store.Subscribe(func(v int) {})
		unsubscribe()
	}
}

func BenchmarkStore_ConcurrentGet(b *testing.B) {
	store := New[testConfig]()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = store.Get()
		}
	})
}

// Phase 3 Tests: Load

func TestStore_Load(t *testing.T) {
	ctx := context.Background()

	t.Run("load single layer", func(t *testing.T) {
		store := New[testConfig]()

		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}), WithPriority(PriorityDefaults))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		err = store.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Host != "localhost" {
			t.Errorf("Host = %q, want %q", cfg.Host, "localhost")
		}
		if cfg.Port != 8080 {
			t.Errorf("Port = %d, want %d", cfg.Port, 8080)
		}
	})

	t.Run("load multiple layers", func(t *testing.T) {
		store := New[testConfig]()

		err := store.Add(mapdata.New("defaults", map[string]any{"host": "localhost", "port": 8080}), WithPriority(PriorityDefaults))
		if err != nil {
			t.Fatalf("Add(defaults) error = %v", err)
		}

		err = store.Add(mapdata.New("user", map[string]any{"port": 9000}), WithPriority(PriorityUser))
		if err != nil {
			t.Fatalf("Add(user) error = %v", err)
		}

		err = store.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Host != "localhost" {
			t.Errorf("Host = %q, want %q", cfg.Host, "localhost")
		}
		if cfg.Port != 9000 {
			t.Errorf("Port = %d, want %d (should be overridden by user layer)", cfg.Port, 9000)
		}
	})

	t.Run("load with no layers", func(t *testing.T) {
		store := New[testConfig]()

		err := store.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Host != "" || cfg.Port != 0 {
			t.Errorf("Get() = %+v, want zero value", cfg)
		}
	})
}

func TestStore_SetTo(t *testing.T) {
	ctx := context.Background()

	t.Run("set value in mapdata layer succeeds", func(t *testing.T) {
		store := New[testConfig]()

		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}), WithPriority(PriorityDefaults))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		err = store.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Set new value should succeed for mapdata layer
		err = store.SetTo("test", "/port", 9000)
		if err != nil {
			t.Fatalf("SetTo() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Port != 9000 {
			t.Errorf("After SetTo() Port = %d, want 9000", cfg.Port)
		}
	})

	t.Run("set notifies subscribers", func(t *testing.T) {
		store := New[testConfig]()

		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}), WithPriority(PriorityDefaults))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		err = store.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		var notified bool
		var receivedPort int
		unsubscribe := store.Subscribe(func(cfg testConfig) {
			notified = true
			receivedPort = cfg.Port
		})
		defer unsubscribe()

		err = store.SetTo("test", "/port", 9000)
		if err != nil {
			t.Fatalf("SetTo() error = %v", err)
		}

		if !notified {
			t.Error("Subscribers were not notified after SetTo()")
		}
		if receivedPort != 9000 {
			t.Errorf("Subscriber received Port = %d, want 9000", receivedPort)
		}
	})

	t.Run("set value in non-existent layer", func(t *testing.T) {
		store := New[testConfig]()

		err := store.SetTo("nonexistent", "/port", 9000)
		if err == nil {
			t.Error("SetTo() should return error for non-existent layer")
		}
	})

	t.Run("set value before load", func(t *testing.T) {
		store := New[testConfig]()

		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost"}), WithPriority(PriorityDefaults))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		// Try to set before loading
		err = store.SetTo("test", "/port", 8080)
		if err == nil {
			t.Error("SetTo() should return error before Load()")
		}
	})
}

func TestStore_GetAt(t *testing.T) {
	ctx := context.Background()
	store := New[testConfig]()

	err := store.Add(mapdata.New("defaults", map[string]any{
		"host": "localhost",
		"port": 8080,
	}), WithPriority(PriorityDefaults))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	t.Run("get existing value", func(t *testing.T) {
		rv := store.GetAt("/host")
		if !rv.Exists {
			t.Error("GetAt(/host).Exists = false, want true")
		}
		if rv.Value != "localhost" {
			t.Errorf("GetAt(/host).Value = %v, want %q", rv.Value, "localhost")
		}
		if rv.Layer.Name() != "defaults" {
			t.Errorf("GetAt(/host).Layer.Name = %q, want %q", rv.Layer.Name(), "defaults")
		}
	})

	t.Run("get non-existent value", func(t *testing.T) {
		rv := store.GetAt("/nonexistent")
		if rv.Exists {
			t.Error("GetAt(/nonexistent).Exists = true, want false")
		}
	})
}

func TestStore_ListLayers(t *testing.T) {
	ctx := context.Background()
	store := New[testConfig]()

	err := store.Add(mapdata.New("defaults", map[string]any{"host": "localhost"}), WithPriority(PriorityDefaults))
	if err != nil {
		t.Fatalf("Add(defaults) error = %v", err)
	}

	err = store.Add(mapdata.New("user", map[string]any{"port": 8080}), WithPriority(PriorityUser))
	if err != nil {
		t.Fatalf("Add(user) error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	layers := store.ListLayers()

	if len(layers) != 2 {
		t.Fatalf("len(ListLayers()) = %d, want 2", len(layers))
	}

	// Check order (should be sorted by priority)
	if layers[0].Name() != "defaults" {
		t.Errorf("layers[0].Name = %q, want %q", layers[0].Name(), "defaults")
	}
	if layers[1].Name() != "user" {
		t.Errorf("layers[1].Name = %q, want %q", layers[1].Name(), "user")
	}

	// Check loaded status
	if !layers[0].Loaded() {
		t.Error("layers[0].Loaded = false, want true")
	}
	if !layers[1].Loaded() {
		t.Error("layers[1].Loaded = false, want true")
	}
}
