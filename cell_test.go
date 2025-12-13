package jubako

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewCell(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		c := NewCell(42)
		if c == nil {
			t.Fatal("NewCell() returned nil")
		}
		if got := c.Get(); got != 42 {
			t.Errorf("Get() = %v, want 42", got)
		}
	})

	t.Run("string", func(t *testing.T) {
		c := NewCell("hello")
		if got := c.Get(); got != "hello" {
			t.Errorf("Get() = %v, want hello", got)
		}
	})

	t.Run("zero value", func(t *testing.T) {
		c := NewCell(0)
		if got := c.Get(); got != 0 {
			t.Errorf("Get() = %v, want 0", got)
		}
	})

	t.Run("struct", func(t *testing.T) {
		type Config struct {
			Host string
			Port int
		}
		cfg := Config{Host: "localhost", Port: 8080}
		c := NewCell(cfg)
		got := c.Get()
		if got.Host != cfg.Host || got.Port != cfg.Port {
			t.Errorf("Get() = %+v, want %+v", got, cfg)
		}
	})
}

func TestCell_Get(t *testing.T) {
	c := NewCell(42)

	// Multiple gets should return the same value
	for i := 0; i < 10; i++ {
		if got := c.Get(); got != 42 {
			t.Errorf("Get() #%d = %v, want 42", i, got)
		}
	}
}

func TestCell_Set(t *testing.T) {
	c := NewCell(0)

	tests := []int{1, 2, 3, 42, 0, -1}
	for _, want := range tests {
		c.Set(want)
		if got := c.Get(); got != want {
			t.Errorf("After Set(%v), Get() = %v", want, got)
		}
	}
}

func TestCell_Subscribe(t *testing.T) {
	t.Run("single subscriber", func(t *testing.T) {
		c := NewCell(0)
		var called int
		var lastValue int

		unsubscribe := c.Subscribe(func(v int) {
			called++
			lastValue = v
		})
		defer unsubscribe()

		c.Set(42)
		if called != 1 {
			t.Errorf("subscriber called %d times, want 1", called)
		}
		if lastValue != 42 {
			t.Errorf("subscriber received %v, want 42", lastValue)
		}

		c.Set(99)
		if called != 2 {
			t.Errorf("subscriber called %d times, want 2", called)
		}
		if lastValue != 99 {
			t.Errorf("subscriber received %v, want 99", lastValue)
		}
	})

	t.Run("multiple subscribers", func(t *testing.T) {
		c := NewCell(0)
		var called1, called2, called3 int

		unsub1 := c.Subscribe(func(v int) { called1++ })
		unsub2 := c.Subscribe(func(v int) { called2++ })
		unsub3 := c.Subscribe(func(v int) { called3++ })
		defer unsub1()
		defer unsub2()
		defer unsub3()

		c.Set(42)
		if called1 != 1 || called2 != 1 || called3 != 1 {
			t.Errorf("subscribers called %d, %d, %d times, want 1, 1, 1", called1, called2, called3)
		}
	})

	t.Run("unsubscribe", func(t *testing.T) {
		c := NewCell(0)
		var called int

		unsubscribe := c.Subscribe(func(v int) { called++ })

		c.Set(1)
		if called != 1 {
			t.Errorf("subscriber called %d times, want 1", called)
		}

		unsubscribe()

		c.Set(2)
		if called != 1 {
			t.Errorf("after unsubscribe, subscriber called %d times, want 1", called)
		}
	})

	t.Run("multiple unsubscribes are safe", func(t *testing.T) {
		c := NewCell(0)
		var called int

		unsubscribe := c.Subscribe(func(v int) { called++ })

		c.Set(1)
		if called != 1 {
			t.Errorf("subscriber called %d times, want 1", called)
		}

		// Call unsubscribe multiple times - should be safe
		unsubscribe()
		unsubscribe()
		unsubscribe()

		c.Set(2)
		if called != 1 {
			t.Errorf("after multiple unsubscribes, subscriber called %d times, want 1", called)
		}
	})

	t.Run("subscriber order", func(t *testing.T) {
		c := NewCell(0)
		var order []int

		c.Subscribe(func(v int) { order = append(order, 1) })
		c.Subscribe(func(v int) { order = append(order, 2) })
		c.Subscribe(func(v int) { order = append(order, 3) })

		c.Set(42)

		if len(order) != 3 {
			t.Fatalf("len(order) = %d, want 3", len(order))
		}
		if order[0] != 1 || order[1] != 2 || order[2] != 3 {
			t.Errorf("subscriber order = %v, want [1, 2, 3]", order)
		}
	})

	t.Run("unsubscribe inside callback does not deadlock", func(t *testing.T) {
		c := NewCell(0)

		done := make(chan struct{})
		var unsubscribe func()
		unsubscribe = c.Subscribe(func(v int) {
			unsubscribe()
			close(done)
		})

		c.Set(1)

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("timeout waiting for callback; possible deadlock")
		}
	})
}

func TestCell_ConcurrentGet(t *testing.T) {
	c := NewCell(42)
	const goroutines = 100
	const iterations = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if got := c.Get(); got != 42 {
					t.Errorf("Get() = %v, want 42", got)
				}
			}
		}()
	}

	wg.Wait()
}

func TestCell_ConcurrentSet(t *testing.T) {
	c := NewCell(0)
	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.Set(id*iterations + j)
			}
		}(i)
	}

	wg.Wait()

	// Final value should be set (no panic, no race)
	_ = c.Get()
}

func TestCell_ConcurrentGetSet(t *testing.T) {
	c := NewCell(0)
	const readers = 50
	const writers = 10
	const duration = 100 * time.Millisecond

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Start readers
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = c.Get()
				}
			}
		}()
	}

	// Start writers
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func(id int) {
			defer wg.Done()
			counter := 0
			for {
				select {
				case <-stop:
					return
				default:
					c.Set(id*1000 + counter)
					counter++
				}
			}
		}(i)
	}

	// Run for duration
	time.Sleep(duration)
	close(stop)
	wg.Wait()
}

func TestCell_ConcurrentSubscribe(t *testing.T) {
	c := NewCell(0)
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Subscribe and unsubscribe concurrently
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			unsubscribe := c.Subscribe(func(v int) {
				// Do nothing
			})
			unsubscribe()
		}()
	}

	wg.Wait()
}

func TestCell_SubscribeWithConcurrentSet(t *testing.T) {
	c := NewCell(0)
	const subscribers = 20
	const setters = 10
	const duration = 100 * time.Millisecond

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var totalCalls int64

	// Start subscribers
	wg.Add(subscribers)
	for i := 0; i < subscribers; i++ {
		go func() {
			defer wg.Done()
			unsubscribe := c.Subscribe(func(v int) {
				atomic.AddInt64(&totalCalls, 1)
			})
			defer unsubscribe()

			<-stop
		}()
	}

	// Give subscribers time to register
	time.Sleep(10 * time.Millisecond)

	// Start setters
	wg.Add(setters)
	for i := 0; i < setters; i++ {
		go func(id int) {
			defer wg.Done()
			counter := 0
			for {
				select {
				case <-stop:
					return
				default:
					c.Set(id*1000 + counter)
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

func TestCell_ReferenceStability(t *testing.T) {
	// This test demonstrates the key feature of Cell:
	// the Cell pointer remains valid even when the value changes

	c := NewCell(42)

	// Get a reference to the Cell
	ref := c

	// Change the value multiple times
	c.Set(100)
	c.Set(200)
	c.Set(300)

	// The reference should still be valid and show the latest value
	if got := ref.Get(); got != 300 {
		t.Errorf("ref.Get() = %v, want 300", got)
	}

	// Subscribe through the reference
	var called int
	unsub := ref.Subscribe(func(v int) { called++ })
	defer unsub()

	c.Set(400)
	if called != 1 {
		t.Errorf("subscriber called %d times, want 1", called)
	}
	if got := ref.Get(); got != 400 {
		t.Errorf("ref.Get() = %v, want 400", got)
	}
}

func BenchmarkCell_Get(b *testing.B) {
	c := NewCell(42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Get()
	}
}

func BenchmarkCell_Set(b *testing.B) {
	c := NewCell(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(i)
	}
}

func BenchmarkCell_SetWithSubscribers(b *testing.B) {
	c := NewCell(0)

	// Add some subscribers
	for i := 0; i < 10; i++ {
		c.Subscribe(func(v int) {
			// Do minimal work
			_ = v
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(i)
	}
}

func BenchmarkCell_ConcurrentGet(b *testing.B) {
	c := NewCell(42)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = c.Get()
		}
	})
}
