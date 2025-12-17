package layer_test

import (
	"testing"
	"time"

	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/watcher"
)

// TestResolveWatchConfig_NoOptions tests that default values are used when no options are provided.
func TestResolveWatchConfig_NoOptions(t *testing.T) {
	cfg := layer.ResolveWatchConfig()

	// Should use default values
	if cfg.PollInterval != watcher.DefaultPollInterval {
		t.Errorf("PollInterval: expected %v, got %v", watcher.DefaultPollInterval, cfg.PollInterval)
	}
	if cfg.CompareFunc == nil {
		t.Error("CompareFunc should not be nil (should be DefaultCompareFunc)")
	}
}

// TestResolveWatchConfig_WithBaseConfigOnly tests that base config values are used.
func TestResolveWatchConfig_WithBaseConfigOnly(t *testing.T) {
	baseCfg := watcher.NewWatchConfig(
		watcher.WithPollInterval(5 * time.Second),
	)

	cfg := layer.ResolveWatchConfig(layer.WithBaseConfig(baseCfg))

	if cfg.PollInterval != 5*time.Second {
		t.Errorf("PollInterval: expected 5s, got %v", cfg.PollInterval)
	}
}

// TestResolveWatchConfig_WithLayerWatchConfigOnly tests that layer options are applied to defaults.
func TestResolveWatchConfig_WithLayerWatchConfigOnly(t *testing.T) {
	cfg := layer.ResolveWatchConfig(
		layer.WithLayerWatchConfig(
			watcher.WithPollInterval(10 * time.Second),
		),
	)

	if cfg.PollInterval != 10*time.Second {
		t.Errorf("PollInterval: expected 10s, got %v", cfg.PollInterval)
	}
}

// TestResolveWatchConfig_LayerOverridesBase tests that layer options override base config.
func TestResolveWatchConfig_LayerOverridesBase(t *testing.T) {
	// Base config: 5 seconds
	baseCfg := watcher.NewWatchConfig(
		watcher.WithPollInterval(5 * time.Second),
	)

	// Layer override: 15 seconds
	cfg := layer.ResolveWatchConfig(
		layer.WithBaseConfig(baseCfg),
		layer.WithLayerWatchConfig(
			watcher.WithPollInterval(15 * time.Second),
		),
	)

	// Layer should win
	if cfg.PollInterval != 15*time.Second {
		t.Errorf("PollInterval: expected 15s (layer override), got %v", cfg.PollInterval)
	}
}

// TestResolveWatchConfig_OptionOrder tests that option order doesn't matter for base vs layer.
func TestResolveWatchConfig_OptionOrder(t *testing.T) {
	baseCfg := watcher.NewWatchConfig(
		watcher.WithPollInterval(5 * time.Second),
	)

	// Test: layer option first, then base config
	cfg := layer.ResolveWatchConfig(
		layer.WithLayerWatchConfig(
			watcher.WithPollInterval(15 * time.Second),
		),
		layer.WithBaseConfig(baseCfg),
	)

	// Layer should still win (layer options are applied after base)
	if cfg.PollInterval != 15*time.Second {
		t.Errorf("PollInterval: expected 15s (layer override), got %v", cfg.PollInterval)
	}
}

// TestResolveWatchConfig_MultipleLayerOptions tests that multiple WithLayerWatchConfig calls accumulate.
func TestResolveWatchConfig_MultipleLayerOptions(t *testing.T) {
	// Custom compare func for testing
	customCompare := func(old, new []byte) bool {
		return len(old) != len(new) // simple length comparison
	}

	cfg := layer.ResolveWatchConfig(
		layer.WithLayerWatchConfig(
			watcher.WithPollInterval(20 * time.Second),
		),
		layer.WithLayerWatchConfig(
			watcher.WithCompareFunc(customCompare),
		),
	)

	// Both options should be applied
	if cfg.PollInterval != 20*time.Second {
		t.Errorf("PollInterval: expected 20s, got %v", cfg.PollInterval)
	}
	if cfg.CompareFunc == nil {
		t.Error("CompareFunc should not be nil")
	}

	// Verify the custom compare func is used
	// Custom func returns true when lengths differ
	if !cfg.CompareFunc([]byte("short"), []byte("longer")) {
		t.Error("CompareFunc should return true for different lengths")
	}
	if cfg.CompareFunc([]byte("same"), []byte("size")) {
		t.Error("CompareFunc should return false for same lengths")
	}
}

// TestResolveWatchConfig_LayerOverridesThenAddMore tests partial override scenario.
func TestResolveWatchConfig_LayerOverridesThenAddMore(t *testing.T) {
	customCompare := func(old, new []byte) bool {
		return true // always changed
	}

	baseCfg := watcher.NewWatchConfig(
		watcher.WithPollInterval(30 * time.Second),
		watcher.WithCompareFunc(watcher.HashCompareFunc),
	)

	cfg := layer.ResolveWatchConfig(
		layer.WithBaseConfig(baseCfg),
		layer.WithLayerWatchConfig(
			watcher.WithPollInterval(5 * time.Second), // Override PollInterval only
		),
		layer.WithLayerWatchConfig(
			watcher.WithCompareFunc(customCompare), // Override CompareFunc too
		),
	)

	// PollInterval should be from layer (5s, not base's 30s)
	if cfg.PollInterval != 5*time.Second {
		t.Errorf("PollInterval: expected 5s (layer override), got %v", cfg.PollInterval)
	}

	// CompareFunc should be from layer (customCompare, not HashCompareFunc)
	if !cfg.CompareFunc([]byte("any"), []byte("thing")) {
		t.Error("CompareFunc should be customCompare (always returns true)")
	}
}

// TestResolveWatchConfig_BaseConfigNotMutated tests that original base config is not mutated.
func TestResolveWatchConfig_BaseConfigNotMutated(t *testing.T) {
	baseCfg := watcher.NewWatchConfig(
		watcher.WithPollInterval(30 * time.Second),
	)
	originalInterval := baseCfg.PollInterval

	// Apply layer override
	_ = layer.ResolveWatchConfig(
		layer.WithBaseConfig(baseCfg),
		layer.WithLayerWatchConfig(
			watcher.WithPollInterval(5 * time.Second),
		),
	)

	// Original base config should not be mutated
	if baseCfg.PollInterval != originalInterval {
		t.Errorf("Base config was mutated: expected %v, got %v", originalInterval, baseCfg.PollInterval)
	}
}

// TestResolveWatchConfig_ZeroValueHandling tests that zero/nil values are replaced with defaults.
func TestResolveWatchConfig_ZeroValueHandling(t *testing.T) {
	// Base config with zero PollInterval and nil CompareFunc
	baseCfg := watcher.WatchConfig{
		PollInterval: 0,
		CompareFunc:  nil,
	}

	cfg := layer.ResolveWatchConfig(layer.WithBaseConfig(baseCfg))

	// Zero/nil values should be replaced with defaults via ApplyDefaults()
	// This ensures watcher implementations can trust config values are valid
	if cfg.PollInterval != watcher.DefaultPollInterval {
		t.Errorf("PollInterval: expected %v (default), got %v", watcher.DefaultPollInterval, cfg.PollInterval)
	}
	if cfg.CompareFunc == nil {
		t.Error("CompareFunc should not be nil (should be DefaultCompareFunc)")
	}
}

// TestResolveWatchConfig_RealWorldScenario tests a realistic Store -> Layer scenario.
func TestResolveWatchConfig_RealWorldScenario(t *testing.T) {
	// Simulate Store creating base config from StoreWatchConfig.WatcherOpts
	storeWatcherOpts := []watcher.WatchConfigOption{
		watcher.WithPollInterval(60 * time.Second), // Store default: 60s
	}
	storeCfg := watcher.NewWatchConfig(storeWatcherOpts...)

	// Simulate Layer.Watch being called with base config
	// and layer having its own overrides
	cfg := layer.ResolveWatchConfig(
		layer.WithBaseConfig(storeCfg),
		layer.WithLayerWatchConfig(
			watcher.WithPollInterval(10 * time.Second), // Layer wants faster polling
		),
	)

	// Layer's preference should be used
	if cfg.PollInterval != 10*time.Second {
		t.Errorf("PollInterval: expected 10s (layer preference), got %v", cfg.PollInterval)
	}
}

// TestResolveWatchConfig_EmptyLayerOptions tests that empty layer options don't affect config.
func TestResolveWatchConfig_EmptyLayerOptions(t *testing.T) {
	baseCfg := watcher.NewWatchConfig(
		watcher.WithPollInterval(25 * time.Second),
	)

	cfg := layer.ResolveWatchConfig(
		layer.WithBaseConfig(baseCfg),
		layer.WithLayerWatchConfig(), // Empty options
	)

	// Base config should be unchanged
	if cfg.PollInterval != 25*time.Second {
		t.Errorf("PollInterval: expected 25s (from base), got %v", cfg.PollInterval)
	}
}
