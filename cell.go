package jubako

import (
	"sync"
	"sync/atomic"
)

// listener wraps a callback function with a unique ID for reliable unsubscription.
type listener[T any] struct {
	id uint64
	fn func(T)
}

// Cell is a reactive container that holds a value and notifies subscribers when it changes.
// It provides reference stability - the Cell pointer remains valid even when the contained value changes.
type Cell[T any] struct {
	value     atomic.Value // stores T
	listeners []listener[T]
	nextID    uint64
	mu        sync.RWMutex
}

// NewCell creates a new Cell with the given initial value.
func NewCell[T any](initial T) *Cell[T] {
	c := &Cell[T]{
		listeners: make([]listener[T], 0),
		nextID:    1,
	}
	c.value.Store(initial)
	return c
}

// Get returns the current value stored in the Cell.
// This operation is lock-free and safe for concurrent use.
func (c *Cell[T]) Get() T {
	return c.value.Load().(T)
}

// Set updates the value in the Cell and notifies all subscribers.
// Subscribers are called synchronously in the order they were registered.
func (c *Cell[T]) Set(v T) {
	c.value.Store(v)

	// Snapshot listeners under lock, then notify without holding the lock.
	// This avoids deadlocks if a listener unsubscribes (or subscribes) within the callback.
	c.mu.RLock()
	listeners := append([]listener[T](nil), c.listeners...)
	c.mu.RUnlock()

	for _, l := range listeners {
		l.fn(v)
	}
}

// Subscribe registers a callback function that will be called whenever the Cell's value changes.
// Returns an unsubscribe function that removes the callback when called.
// The unsubscribe function is safe to call multiple times.
//
// Example:
//
//	cell := NewCell(42)
//	unsubscribe := cell.Subscribe(func(v int) {
//	  fmt.Println("Value changed:", v)
//	})
//	defer unsubscribe()
func (c *Cell[T]) Subscribe(fn func(T)) func() {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextID
	c.nextID++
	c.listeners = append(c.listeners, listener[T]{id: id, fn: fn})

	// Return unsubscribe function
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		for i, l := range c.listeners {
			if l.id == id {
				c.listeners = append(c.listeners[:i], c.listeners[i+1:]...)
				return
			}
		}
	}
}
