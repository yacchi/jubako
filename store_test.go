package jubako

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yacchi/jubako/document"
	jjson "github.com/yacchi/jubako/format/json"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/layer/mapdata"
	"github.com/yacchi/jubako/source"
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

// Helper types for additional Store tests

type noSaveLayer struct{ name layer.Name }

func (l *noSaveLayer) Name() layer.Name { return l.name }
func (l *noSaveLayer) Load(ctx context.Context) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"a": 1}, nil
}
func (l *noSaveLayer) Save(context.Context, document.JSONPatchSet) error {
	return source.ErrSaveNotSupported
}
func (l *noSaveLayer) CanSave() bool { return false }

type pathMemSource struct {
	path    string
	data    []byte
	canSave bool
}

func (s *pathMemSource) Path() string { return s.path }
func (s *pathMemSource) Load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b := make([]byte, len(s.data))
	copy(b, s.data)
	return b, nil
}
func (s *pathMemSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.canSave {
		return source.ErrSaveNotSupported
	}
	newData, err := updateFunc(s.data)
	if err != nil {
		return err
	}
	s.data = newData
	return nil
}
func (s *pathMemSource) CanSave() bool { return s.canSave }

type failingSaveLayer struct{ name layer.Name }

func (l *failingSaveLayer) Name() layer.Name { return l.name }
func (l *failingSaveLayer) Load(ctx context.Context) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"a": 1}, nil
}
func (l *failingSaveLayer) Save(context.Context, document.JSONPatchSet) error {
	return errors.New("save error")
}
func (l *failingSaveLayer) CanSave() bool { return true }

type nilDataLayer struct{ name layer.Name }

func (l *nilDataLayer) Name() layer.Name { return l.name }
func (l *nilDataLayer) Load(ctx context.Context) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}
func (l *nilDataLayer) Save(context.Context, document.JSONPatchSet) error { return nil }
func (l *nilDataLayer) CanSave() bool                                     { return true }

func TestStore_ReadOnlyAndLayerInfo(t *testing.T) {
	ctx := context.Background()
	store := New[map[string]any]()

	if err := store.Add(mapdata.New("ro", map[string]any{"a": 1}), WithReadOnly()); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	info := store.GetLayerInfo("ro")
	if info == nil {
		t.Fatal("GetLayerInfo() returned nil")
	}
	if got := info.Format(); got != "" {
		t.Fatalf("Format() = %q, want empty string for non-format layers", got)
	}
	if !info.ReadOnly() {
		t.Fatal("ReadOnly() = false, want true")
	}
	if info.Writable() {
		t.Fatal("Writable() = true, want false")
	}
	if info.Dirty() {
		t.Fatal("Dirty() = true, want false")
	}

	if err := store.SetTo("ro", "/a", 2); err == nil {
		t.Fatal("SetTo() on read-only layer expected error, got nil")
	}
}

func TestStore_WithDecoder_AndSaveDirty(t *testing.T) {
	ctx := context.Background()
	var decoderCalls int
	store := New[testConfig](WithDecoder(func(data map[string]any, target any) error {
		decoderCalls++
		m, ok := target.(*testConfig)
		if !ok {
			return errors.New("unexpected target type")
		}
		m.Host, _ = data["host"].(string)
		if port, ok := data["port"].(int); ok {
			m.Port = port
		} else if port, ok := data["port"].(float64); ok {
			m.Port = int(port)
		}
		return nil
	}))

	l := mapdata.New("user", map[string]any{"host": "h", "port": 1})
	if err := store.Add(l, WithPriority(PriorityUser)); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if decoderCalls == 0 {
		t.Fatal("expected decoder to be called at least once")
	}

	if store.IsDirty() {
		t.Fatal("IsDirty() = true, want false")
	}
	if err := store.SetTo("user", "/port", 2); err != nil {
		t.Fatalf("SetTo() error = %v", err)
	}
	if !store.IsDirty() {
		t.Fatal("IsDirty() = false, want true")
	}

	// Save all dirty layers.
	if err := store.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if store.IsDirty() {
		t.Fatal("IsDirty() = true after Save, want false")
	}

	// SaveLayer on a missing layer.
	if err := store.SaveLayer(ctx, "missing"); err == nil {
		t.Fatal("SaveLayer(missing) expected error, got nil")
	}
}

func TestStore_Load_DecodeError(t *testing.T) {
	ctx := context.Background()
	store := New[testConfig](WithDecoder(func(map[string]any, any) error {
		return errors.New("decode error")
	}))
	if err := store.Add(mapdata.New("user", map[string]any{"host": "h"})); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Load(ctx); err == nil {
		t.Fatal("Load() expected error, got nil")
	}
}

func TestStore_SaveLayer_ErrorPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("layer not loaded", func(t *testing.T) {
		store := New[testConfig]()
		if err := store.Add(mapdata.New("user", map[string]any{"host": "h"})); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.SaveLayer(ctx, "user"); err == nil {
			t.Fatal("SaveLayer() expected error, got nil")
		}
	})

	t.Run("layer does not support saving", func(t *testing.T) {
		store := New[testConfig]()
		if err := store.Add(&noSaveLayer{name: "nosave"}); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		// SaveLayer is a no-op when there are no pending changes.
		if err := store.SaveLayer(ctx, "nosave"); err != nil {
			t.Fatalf("SaveLayer() error = %v", err)
		}

		// Force the layer into a dirty state to validate the error path.
		func() {
			store.mu.Lock()
			defer store.mu.Unlock()
			entry := store.findLayerLocked("nosave")
			entry.dirty = true
			entry.changeset = document.JSONPatchSet{
				{Op: document.PatchOpReplace, Path: "/host", Value: "h2"},
			}
		}()

		if err := store.SaveLayer(ctx, "nosave"); err == nil {
			t.Fatal("SaveLayer() expected error, got nil")
		}
	})

	t.Run("read-only layer cannot be saved", func(t *testing.T) {
		store := New[testConfig]()
		if err := store.Add(mapdata.New("ro", map[string]any{"host": "h"}), WithReadOnly()); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Force the layer into a dirty state to validate the error path.
		func() {
			store.mu.Lock()
			defer store.mu.Unlock()
			entry := store.findLayerLocked("ro")
			entry.dirty = true
			entry.changeset = document.JSONPatchSet{
				{Op: document.PatchOpReplace, Path: "/host", Value: "h2"},
			}
		}()

		if err := store.SaveLayer(ctx, "ro"); err == nil {
			t.Fatal("SaveLayer() on read-only layer expected error, got nil")
		}
	})
}

func TestStore_Reload_ReappliesChangeset(t *testing.T) {
	ctx := context.Background()
	store := New[testConfig]()

	if err := store.Add(mapdata.New("user", map[string]any{"host": "h", "port": 1}), WithPriority(PriorityUser)); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Add(mapdata.New("other", map[string]any{"host": "other"}), WithPriority(PriorityDefaults)); err != nil {
		t.Fatalf("Add(other) error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := store.SetTo("user", "/port", 2); err != nil {
		t.Fatalf("SetTo() error = %v", err)
	}

	// Reload should preserve the in-memory changeset and reapply it.
	if err := store.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if got := store.Get().Port; got != 2 {
		t.Fatalf("after Reload, Port = %d, want 2", got)
	}
}

func TestStore_Reload_ReappliesRemovePatch(t *testing.T) {
	ctx := context.Background()
	store := New[testConfig]()

	if err := store.Add(mapdata.New("user", map[string]any{"host": "h", "port": 1}), WithPriority(PriorityUser)); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	func() {
		store.mu.Lock()
		defer store.mu.Unlock()
		entry := store.findLayerLocked("user")
		entry.changeset = []document.JSONPatch{
			{Op: document.PatchOpRemove, Path: "/host"},
		}
		entry.dirty = true
	}()

	if err := store.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if got := store.Get().Host; got != "" {
		t.Fatalf("after Reload remove, Host = %q, want empty", got)
	}
}

func TestStore_Reload_ReappliesAddPatch(t *testing.T) {
	ctx := context.Background()
	store := New[testConfig]()

	if err := store.Add(mapdata.New("user", map[string]any{"host": "h", "port": 1}), WithPriority(PriorityUser)); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	// Create a new key so the changeset contains an "add" operation.
	if err := store.SetTo("user", "/new_key", "x"); err != nil {
		t.Fatalf("SetTo() error = %v", err)
	}

	if err := store.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
}

func TestStore_Reload_NotifiesSubscribers(t *testing.T) {
	ctx := context.Background()
	store := New[testConfig]()

	if err := store.Add(mapdata.New("user", map[string]any{"host": "h", "port": 1}), WithPriority(PriorityUser)); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	called := 0
	unsub := store.Subscribe(func(testConfig) { called++ })
	defer unsub()

	if err := store.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if called == 0 {
		t.Fatal("expected subscriber to be called on Reload")
	}
}

func TestStore_Reload_MaterializeError(t *testing.T) {
	ctx := context.Background()
	store := New[testConfig]()

	if err := store.Add(mapdata.New("user", map[string]any{"host": "h", "port": 1}), WithPriority(PriorityUser)); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	func() {
		store.mu.Lock()
		defer store.mu.Unlock()
		store.decoder = func(map[string]any, any) error { return errors.New("decode error") }
	}()

	if err := store.Reload(ctx); err == nil {
		t.Fatal("Reload() expected error, got nil")
	}
}

func TestStore_Save_NoDirty(t *testing.T) {
	ctx := context.Background()
	store := New[testConfig]()
	if err := store.Add(mapdata.New("user", map[string]any{"host": "h"})); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := store.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestStore_Save_ErrorFromLayer(t *testing.T) {
	ctx := context.Background()
	store := New[map[string]any]()
	if err := store.Add(&failingSaveLayer{name: "bad"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := store.SetTo("bad", "/a", 2); err != nil {
		t.Fatalf("SetTo() error = %v", err)
	}
	if err := store.Save(ctx); err == nil {
		t.Fatal("Save() expected error, got nil")
	}
	if !store.IsDirty() {
		t.Fatal("IsDirty() = false after failed Save, want true")
	}
}

func TestStore_SetTo_ErrorPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("missing layer", func(t *testing.T) {
		store := New[testConfig]()
		if err := store.SetTo("missing", "/a", 1); err == nil {
			t.Fatal("SetTo(missing) expected error, got nil")
		}
	})

	t.Run("layer not loaded", func(t *testing.T) {
		store := New[testConfig]()
		if err := store.Add(mapdata.New("user", map[string]any{"a": 1})); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.SetTo("user", "/a", 2); err == nil {
			t.Fatal("SetTo() expected error, got nil")
		}
	})

	t.Run("layer not writable", func(t *testing.T) {
		store := New[testConfig]()
		if err := store.Add(&noSaveLayer{name: "nosave"}); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if err := store.SetTo("nosave", "/a", 2); err == nil {
			t.Fatal("SetTo() expected error, got nil")
		}
	})

	t.Run("invalid pointer", func(t *testing.T) {
		store := New[testConfig]()
		if err := store.Add(mapdata.New("user", map[string]any{"a": 1})); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if err := store.SetTo("user", "relative/path", 2); err == nil {
			t.Fatal("SetTo(relative) expected error, got nil")
		}
		if err := store.SetTo("user", "", 2); err == nil {
			t.Fatal("SetTo(empty) expected error, got nil")
		}
	})

	t.Run("created vs replaced", func(t *testing.T) {
		store := New[map[string]any]()
		if err := store.Add(mapdata.New("user", map[string]any{"a": 1})); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if err := store.SetTo("user", "/b", 2); err != nil {
			t.Fatalf("SetTo(create) error = %v", err)
		}
		if err := store.SetTo("user", "/b", 3); err != nil {
			t.Fatalf("SetTo(replace) error = %v", err)
		}
	})
}

func TestStore_SetTo_MaterializeError(t *testing.T) {
	ctx := context.Background()
	store := New[testConfig]()

	if err := store.Add(mapdata.New("user", map[string]any{"host": "h", "port": 1})); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	func() {
		store.mu.Lock()
		defer store.mu.Unlock()
		store.decoder = func(map[string]any, any) error { return errors.New("decode error") }
	}()

	if err := store.SetTo("user", "/host", "x"); err == nil {
		t.Fatal("SetTo() expected error, got nil")
	}
}

func TestStore_GetLayer_GetAtContainers_GetAllAt(t *testing.T) {
	ctx := context.Background()
	store := New[map[string]any]()

	if err := store.Add(mapdata.New("base", map[string]any{
		"server": map[string]any{"host": "a", "port": 1},
		"items":  []any{"a"},
		"complex": []any{
			map[string]any{"x": 1},
			[]any{"y"},
			"z",
		},
	}), WithPriority(0)); err != nil {
		t.Fatalf("Add(base) error = %v", err)
	}
	if err := store.Add(mapdata.New("override", map[string]any{
		"server": map[string]any{"port": 2},
		"items":  []any{"b"},
	}), WithPriority(10)); err != nil {
		t.Fatalf("Add(override) error = %v", err)
	}

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !store.origins.isContainer("/server") {
		t.Fatal("origins.isContainer(/server) = false, want true")
	}

	// GetLayer returns the registered layer.
	if got := store.GetLayer("base"); got == nil {
		t.Fatal("GetLayer(base) = nil")
	}
	if got := store.GetLayer("missing"); got != nil {
		t.Fatal("GetLayer(missing) != nil")
	}

	// Container GetAt merges maps across layers and uses highest-priority origin.
	rv := store.GetAt("/server")
	if !rv.Exists {
		t.Fatal("GetAt(/server) Exists=false, want true")
	}
	gotMap, ok := rv.Value.(map[string]any)
	if !ok {
		t.Fatalf("GetAt(/server).Value is %T, want map[string]any", rv.Value)
	}
	wantMap := map[string]any{"host": "a", "port": 2}
	if !reflect.DeepEqual(gotMap, wantMap) {
		t.Fatalf("GetAt(/server) = %#v, want %#v", gotMap, wantMap)
	}
	if rv.Layer == nil || rv.Layer.Name() != "override" {
		t.Fatalf("origin layer = %v, want %q", rv.Layer, "override")
	}

	// Slice containers replace on higher priority.
	rv = store.GetAt("/items")
	if !rv.Exists {
		t.Fatal("GetAt(/items) Exists=false, want true")
	}
	if !reflect.DeepEqual(rv.Value, []any{"b"}) {
		t.Fatalf("GetAt(/items) = %#v, want %#v", rv.Value, []any{"b"})
	}

	// GetAllAt returns values from all contributing layers (leaf and container paths).
	all := store.GetAllAt("/server/port")
	if all.Len() != 2 {
		t.Fatalf("GetAllAt(/server/port).Len() = %d, want 2", all.Len())
	}
	if eff := all.Effective(); !eff.Exists || eff.Layer.Name() != "override" {
		t.Fatalf("Effective() = %#v", eff)
	}

	all = store.GetAllAt("/server")
	if all.Len() != 2 {
		t.Fatalf("GetAllAt(/server).Len() = %d, want 2", all.Len())
	}

	if got := store.GetAllAt("/missing"); got != nil {
		t.Fatalf("GetAllAt(/missing) = %#v, want nil", got)
	}
}

func TestStore_LayerInfo_FormatAndPath(t *testing.T) {
	ctx := context.Background()
	store := New[map[string]any]()

	src := &pathMemSource{
		path:    "/tmp/config.json",
		data:    []byte("{\"a\": 1}\n"),
		canSave: true,
	}
	doc := jjson.New()
	if err := store.Add(layer.New("file", src, doc)); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	info := store.GetLayerInfo("file")
	if info == nil {
		t.Fatal("GetLayerInfo() returned nil")
	}
	if got := info.Path(); got != "/tmp/config.json" {
		t.Fatalf("Path() = %q, want %q", got, "/tmp/config.json")
	}
	if got := info.Format(); got != document.FormatJSON {
		t.Fatalf("Format() = %q, want %q", got, document.FormatJSON)
	}
	if !info.Writable() {
		t.Fatal("Writable() = false, want true")
	}
}

func TestOriginsAndResolvedValues_EdgeCases(t *testing.T) {
	var o *origin
	if got := o.get(); got != nil {
		t.Fatalf("(*origin)(nil).get() = %v, want nil", got)
	}
	if got := o.getAll(); got != nil {
		t.Fatalf("(*origin)(nil).getAll() = %v, want nil", got)
	}

	if got := (ResolvedValues(nil)).Effective(); got.Exists {
		t.Fatalf("ResolvedValues(nil).Effective() = %#v, want empty", got)
	}

	if got := newResolvedValue(nil, "/a"); got.Exists {
		t.Fatalf("newResolvedValue(nil) = %#v, want empty", got)
	}
	entry := &layerEntry{data: nil}
	if got := newResolvedValue(entry, "/a"); got.Exists {
		t.Fatalf("newResolvedValue(data=nil) = %#v, want empty", got)
	}
	entry = &layerEntry{data: map[string]any{"a": 1}}
	if got := newResolvedValue(entry, "/missing"); got.Exists {
		t.Fatalf("newResolvedValue(missing path) = %#v, want empty", got)
	}

	o2 := newOrigins()
	if got := o2.getAllContainer("/missing"); got != nil {
		t.Fatalf("getAllContainer(/missing) = %#v, want nil", got)
	}
	if got := o2.isContainer("/missing"); got {
		t.Fatalf("isContainer(/missing) = true, want false")
	}
}

func TestStore_GetLayerInfo_NotFound(t *testing.T) {
	store := New[testConfig]()
	if got := store.GetLayerInfo("missing"); got != nil {
		t.Fatalf("GetLayerInfo(missing) = %v, want nil", got)
	}
}

func TestStore_LoadReload_ErrorPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("Load error from layer", func(t *testing.T) {
		store := New[testConfig]()
		if err := store.Add(&noSaveLayer{name: "bad"}); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		// Force an error from Load via canceled context.
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if err := store.Load(canceled); err == nil {
			t.Fatal("Load() expected error, got nil")
		}
	})

	t.Run("Reload error from layer", func(t *testing.T) {
		store := New[testConfig]()
		if err := store.Add(mapdata.New("ok", map[string]any{"host": "h"})); err != nil {
			t.Fatalf("Add(ok) error = %v", err)
		}
		if err := store.Add(&noSaveLayer{name: "bad"}); err != nil {
			t.Fatalf("Add(bad) error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if err := store.Reload(canceled); err == nil {
			t.Fatal("Reload() expected error, got nil")
		}
	})
}

func TestMappingTable_IsEmpty_NilReceiver(t *testing.T) {
	var t0 *MappingTable
	if !t0.IsEmpty() {
		t.Fatal("(*MappingTable)(nil).IsEmpty() = false, want true")
	}
}

func TestMappingTable_String_NilReceiver(t *testing.T) {
	var t0 *MappingTable
	if got, want := t0.String(), "(no mappings)"; got != want {
		t.Fatalf("(*MappingTable)(nil).String() = %q, want %q", got, want)
	}
}

func TestParseJubakoTag(t *testing.T) {
	tests := []struct {
		in       string
		wantPath string
		wantRel  bool
	}{
		{"", "", false},
		{"/a/b", "/a/b", false},
		{"a/b", "/a/b", true},
		{"./a/b", "/a/b", true},
		{"a/b,option", "/a/b", true},
	}
	for _, tt := range tests {
		gotPath, gotRel := parseJubakoTag(tt.in)
		if gotPath != tt.wantPath || gotRel != tt.wantRel {
			t.Fatalf("parseJubakoTag(%q) = (%q, %v), want (%q, %v)", tt.in, gotPath, gotRel, tt.wantPath, tt.wantRel)
		}
	}
}

func TestWalkContext_AllValues_EmptyOrigin(t *testing.T) {
	c := WalkContext{Path: "/x", origin: &origin{}}
	if got := c.AllValues(); got != nil {
		t.Fatalf("AllValues() = %#v, want nil", got)
	}
}

func TestWalkContext_ValueAndAllValues(t *testing.T) {
	ctx := context.Background()
	store := New[map[string]any]()

	if err := store.Add(mapdata.New("base", map[string]any{"a": nil}), WithPriority(0)); err != nil {
		t.Fatalf("Add(base) error = %v", err)
	}
	if err := store.Add(mapdata.New("override", map[string]any{"a": "x"}), WithPriority(10)); err != nil {
		t.Fatalf("Add(override) error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	var saw bool
	store.Walk(func(c WalkContext) bool {
		if c.Path != "/a" {
			return true
		}
		saw = true

		// Exercise ResolvedValue helpers via WalkContext.Value().
		v := c.Value()
		if !v.HasValue() {
			t.Fatalf("Value().HasValue() = false, want true")
		}
		if v.IsNull() {
			t.Fatalf("Value().IsNull() = true, want false")
		}
		if v.IsMissing() {
			t.Fatalf("Value().IsMissing() = true, want false")
		}

		// Exercise WalkContext.AllValues() and Effective().
		all := c.AllValues()
		if all.Len() != 2 {
			t.Fatalf("AllValues().Len() = %d, want 2", all.Len())
		}
		if eff := all.Effective(); eff.Layer == nil || eff.Layer.Name() != "override" {
			t.Fatalf("Effective() = %#v", eff)
		}
		return false
	})
	if !saw {
		t.Fatal("did not observe expected path in Walk")
	}
}

func TestParseJSONTagKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"name", "name"},
		{"name,omitempty", "name"},
		{"-", "-"},
	}
	for _, tt := range tests {
		if got := parseJSONTagKey(tt.in); got != tt.want {
			t.Fatalf("parseJSONTagKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildMappingTable_ComplexTypes(t *testing.T) {
	type inner struct {
		Value string `json:"value" jubako:"./x"`
	}
	type cfg struct {
		A       string            `json:"a" jubako:"/root/a"`
		B       string            `jubako:"b"`              // no json tag
		C       string            `json:",omitempty"`       // empty json key falls back to field name
		Skip    string            `json:"skip" jubako:"-"`  // explicitly skipped
		Ignore  string            `json:"-" jubako:"/gone"` // ignored by json tag
		In      inner             `json:"in"`
		Ptr     *inner            `json:"ptr"`
		PtrList []*inner          `json:"ptr_list"`
		List    []inner           `json:"list"`
		Dict    map[string]inner  `json:"dict"`
		PtrDict map[string]*inner `json:"ptr_dict"`
		private string
	}

	if got := buildMappingTable(reflect.TypeOf(0)); got != nil {
		t.Fatalf("buildMappingTable(non-struct) = %v, want nil", got)
	}

	table := buildMappingTable(reflect.TypeOf(cfg{}))
	if table == nil || table.IsEmpty() {
		t.Fatal("buildMappingTable() returned empty table")
	}
	if s := table.String(); s == "" {
		t.Fatal("MappingTable.String() returned empty")
	}

	table = buildMappingTable(reflect.TypeOf(&cfg{}))
	if table == nil || table.IsEmpty() {
		t.Fatal("buildMappingTable(ptr) returned empty table")
	}
}

func TestApplyMappingsWithRoot_SliceAndMapFallbacks(t *testing.T) {
	type inner struct {
		Value string `json:"value" jubako:"./x"`
	}
	type cfg struct {
		List []inner          `json:"list"`
		Dict map[string]inner `json:"dict"`
	}

	table := buildMappingTable(reflect.TypeOf(cfg{}))
	src := map[string]any{
		"list": []any{
			"not-a-map",
			map[string]any{"x": "ok"},
		},
		"dict": map[string]any{
			"a": map[string]any{"x": "ok"},
			"b": "not-a-map",
		},
	}

	out := applyMappingsWithRoot(src, src, table)
	list := out["list"].([]any)
	if list[0] != "not-a-map" {
		t.Fatalf("list[0] = %#v", list[0])
	}
	if got := list[1].(map[string]any)["value"]; got != "ok" {
		t.Fatalf("list[1].value = %#v, want %q", got, "ok")
	}
	dict := out["dict"].(map[string]any)
	if got := dict["a"].(map[string]any)["value"]; got != "ok" {
		t.Fatalf("dict[a].value = %#v, want %q", got, "ok")
	}
	if dict["b"] != "not-a-map" {
		t.Fatalf("dict[b] = %#v", dict["b"])
	}
}

func TestStore_Load_SkipsNilLayerData(t *testing.T) {
	ctx := context.Background()
	store := New[testConfig]()
	if err := store.Add(&nilDataLayer{name: "nil"}); err != nil {
		t.Fatalf("Add(nil) error = %v", err)
	}
	if err := store.Add(mapdata.New("user", map[string]any{"host": "h", "port": 1})); err != nil {
		t.Fatalf("Add(user) error = %v", err)
	}
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := store.Get().Host; got != "h" {
		t.Fatalf("Host = %q, want %q", got, "h")
	}
}

func TestResolveContainerLocked_EdgeCases(t *testing.T) {
	store := New[map[string]any]()

	// No entries.
	if got := store.resolveContainerLocked("/missing", &layerEntry{}); got.Exists {
		t.Fatalf("resolveContainerLocked(/missing) = %#v, want empty", got)
	}

	// Entries exist but none contribute a value.
	nilEntry := &layerEntry{data: nil}
	missingEntry := &layerEntry{data: map[string]any{"a": 1}}
	store.origins.setContainer("/c", nilEntry)
	store.origins.setContainer("/c", missingEntry)
	if got := store.resolveContainerLocked("/c", missingEntry); got.Exists {
		t.Fatalf("resolveContainerLocked(/c) = %#v, want empty", got)
	}
}

func TestStore_GetAllAt_SkipsEmptyResolvedValues(t *testing.T) {
	store := New[map[string]any]()
	nilEntry := &layerEntry{data: nil}
	store.origins.setLeaf("/a", nilEntry)
	if got := store.GetAllAt("/a"); got == nil || got.Len() != 0 {
		t.Fatalf("GetAllAt(/a) = %#v, want empty slice", got)
	}
}
