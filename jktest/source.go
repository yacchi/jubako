// Package jktest provides testing utilities for jubako implementations.
package jktest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
)

// testInitParams creates WatcherInitializerParams for testing.
// Uses a dummy fetch function and a real mutex.
func testInitParams() watcher.WatcherInitializerParams {
	return testInitParamsWithConfig(watcher.NewWatchConfig())
}

func testInitParamsWithConfig(cfg watcher.WatchConfig) watcher.WatcherInitializerParams {
	var mu sync.Mutex
	return watcher.WatcherInitializerParams{
		Fetch: func(ctx context.Context) (bool, []byte, error) {
			return true, nil, nil
		},
		OpMu:   &mu,
		Config: cfg,
	}
}

// SourceFactory creates a Source initialized with the given test data.
// The factory is called for each test case to ensure test isolation.
type SourceFactory func(data []byte) source.Source

// NotExistFactory creates a Source that points to a non-existent resource.
// This is used to test ErrNotExist handling for NotExistCapable sources.
type NotExistFactory func() source.Source

// SourceTesterOption configures SourceTester behavior.
type SourceTesterOption func(*SourceTester)

// WithNotExistFactory sets a factory that creates a Source for a non-existent resource.
// This is required for sources that implement NotExistCapable.
// If not provided and the source implements NotExistCapable, a warning will be logged.
func WithNotExistFactory(factory NotExistFactory) SourceTesterOption {
	return func(st *SourceTester) {
		st.notExistFactory = factory
	}
}

// SourceTester provides utilities to verify Source implementations.
type SourceTester struct {
	t              *testing.T
	factory        SourceFactory
	notExistFactory NotExistFactory
}

// NewSourceTester creates a SourceTester for the given SourceFactory.
// The factory will be used to create new Source instances for each test.
func NewSourceTester(t *testing.T, factory SourceFactory, opts ...SourceTesterOption) *SourceTester {
	st := &SourceTester{
		t:       t,
		factory: factory,
	}
	for _, opt := range opts {
		opt(st)
	}
	return st
}

// TestAll runs all standard compliance tests for Source implementations.
func (st *SourceTester) TestAll() {
	st.t.Run("Type", st.testType)
	st.t.Run("Load", st.testLoad)
	st.t.Run("CanSave", st.testCanSave)
	st.t.Run("Watch", st.testWatch)
	st.t.Run("NotExistCapable", st.testNotExistCapable)
}

// testType verifies Type() returns a non-empty SourceType.
func (st *SourceTester) testType(t *testing.T) {
	s := st.factory([]byte(`{"key": "value"}`))

	typ := s.Type()
	require(t, typ != "", "Type() returned empty string")
}

// testLoad verifies Load() returns the correct data.
func (st *SourceTester) testLoad(t *testing.T) {
	testData := []byte(`{"key": "value"}`)
	s := st.factory(testData)

	data, err := s.Load(context.Background())
	requireNoError(t, err, "Load error = %v", err)
	require(t, len(data) > 0, "Load returned empty data")
}

// testCanSave verifies CanSave() is consistent with Save() behavior.
func (st *SourceTester) testCanSave(t *testing.T) {
	s := st.factory([]byte(`{"key": "value"}`))

	canSave := s.CanSave()
	err := s.Save(context.Background(), func(current []byte) ([]byte, error) {
		return []byte(`{"key": "new_value"}`), nil
	})

	if canSave {
		// If CanSave() returns true, Save() should not return ErrSaveNotSupported
		check(t, err != source.ErrSaveNotSupported,
			"CanSave() returned true but Save() returned ErrSaveNotSupported")
	} else {
		// If CanSave() returns false, Save() should return ErrSaveNotSupported
		check(t, err == source.ErrSaveNotSupported,
			"CanSave() returned false but Save() did not return ErrSaveNotSupported, got %v", err)
	}
}

// testWatch verifies Watch behavior for WatchableSource implementations.
func (st *SourceTester) testWatch(t *testing.T) {
	s := st.factory([]byte(`{"key": "value"}`))

	ws, ok := s.(source.WatchableSource)
	if !ok {
		t.Skip("Source does not implement WatchableSource")
		return
	}

	t.Run("WatchReturnsValidWatcher", func(t *testing.T) {
		init, err := ws.Watch()
		requireNoError(t, err, "Watch() error = %v", err)
		require(t, init != nil, "Watch() returned nil initializer")

		// Create watcher with test params
		w, err := init(testInitParams())
		requireNoError(t, err, "WatcherInitializer() error = %v", err)
		require(t, w != nil, "WatcherInitializer() returned nil watcher")

		// Verify watcher type is valid
		typ := w.Type()
		require(t, typ != "", "Watcher.Type() returned empty string")
		check(t, typ == watcher.TypePolling || typ == watcher.TypeSubscription || typ == watcher.TypeNoop,
			"Watcher.Type() returned unknown type: %q", typ)
	})

	t.Run("WatchDoesNotStartUntilStartCalled", func(t *testing.T) {
		init, err := ws.Watch()
		requireNoError(t, err, "Watch() error = %v", err)

		w, err := init(testInitParams())
		requireNoError(t, err, "WatcherInitializer() error = %v", err)

		// Results() should return nil before Start() is called
		results := w.Results()
		check(t, results == nil, "Results() should return nil before Start() is called, got %v", results)
	})

	t.Run("WatchStartStop", func(t *testing.T) {
		init, err := ws.Watch()
		requireNoError(t, err, "Watch() error = %v", err)

		cfg := watcher.NewWatchConfig(
			watcher.WithPollInterval(100 * time.Millisecond),
		)
		w, err := init(testInitParamsWithConfig(cfg))
		requireNoError(t, err, "WatcherInitializer() error = %v", err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = w.Start(ctx)
		requireNoError(t, err, "Start() error = %v", err)

		// Results() should return a non-nil channel after Start()
		results := w.Results()
		require(t, results != nil, "Results() returned nil after Start()")

		// Stop should work without error
		err = w.Stop(context.Background())
		requireNoError(t, err, "Stop() error = %v", err)
	})

	t.Run("WatchCanBeCalledMultipleTimes", func(t *testing.T) {
		// Calling Watch() multiple times should not cause issues
		init1, err := ws.Watch()
		requireNoError(t, err, "First Watch() error = %v", err)
		require(t, init1 != nil, "First Watch() returned nil initializer")

		init2, err := ws.Watch()
		requireNoError(t, err, "Second Watch() error = %v", err)
		require(t, init2 != nil, "Second Watch() returned nil initializer")

		w1, err := init1(testInitParams())
		requireNoError(t, err, "First WatcherInitializer() error = %v", err)
		w2, err := init2(testInitParams())
		requireNoError(t, err, "Second WatcherInitializer() error = %v", err)

		// Both watchers should have valid types
		check(t, w1.Type() != "", "First watcher Type() returned empty string")
		check(t, w2.Type() != "", "Second watcher Type() returned empty string")
	})
}

// testNotExistCapable verifies NotExistCapable implementations.
// If the source implements NotExistCapable and CanNotExist() returns true,
// this test verifies that ErrNotExist is properly returned for missing resources.
func (st *SourceTester) testNotExistCapable(t *testing.T) {
	s := st.factory([]byte(`{"key": "value"}`))

	nc, ok := s.(source.NotExistCapable)
	if !ok {
		t.Log("Source does not implement NotExistCapable (this is OK for sources like bytes.Source)")
		return
	}

	if !nc.CanNotExist() {
		t.Log("Source implements NotExistCapable but CanNotExist() returns false")
		return
	}

	// Source declares it can return ErrNotExist
	t.Run("CanNotExistIsTrue", func(t *testing.T) {
		require(t, nc.CanNotExist(), "CanNotExist() should return true")
	})

	t.Run("LoadReturnsErrNotExist", func(t *testing.T) {
		if st.notExistFactory == nil {
			t.Log("WARNING: Source implements NotExistCapable with CanNotExist()=true, " +
				"but no NotExistFactory was provided. " +
				"Use jktest.WithNotExistFactory() to enable ErrNotExist testing.")
			t.Skip("NotExistFactory not provided")
			return
		}

		// Create a source pointing to a non-existent resource
		notExistSource := st.notExistFactory()

		// Load should return an error wrapping source.ErrNotExist
		_, err := notExistSource.Load(context.Background())
		require(t, err != nil, "Load() on non-existent resource should return error")
		check(t, errors.Is(err, source.ErrNotExist),
			"Load() error should wrap source.ErrNotExist, got: %v", err)
	})
}
