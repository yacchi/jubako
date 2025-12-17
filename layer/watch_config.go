package layer

import "github.com/yacchi/jubako/watcher"

// WatchOption configures layer-level watch behavior.
type WatchOption func(*watchOptions)

// watchOptions holds configuration for layer watching.
//
// Configuration is applied in two stages:
//  1. Store passes base config via WithBaseConfig (from StoreWatchConfig.WatcherOpts)
//  2. Layer-level overrides are applied via WithLayerWatchConfig (optional)
//
// Use resolveConfig() to get the final merged configuration.
type watchOptions struct {
	// baseConfig is the base configuration from Store.
	// If nil, defaults will be used when resolveConfig() is called.
	baseConfig *watcher.WatchConfig

	// configOpts are layer-level overrides applied after baseConfig.
	// These allow individual layers to customize behavior (e.g., different poll interval).
	configOpts []watcher.WatchConfigOption
}

// resolveConfig merges baseConfig with layer-level options and returns the final config.
// If baseConfig is nil, starts with default values.
//
// This method makes the config merging explicit and traceable:
//  1. Base: baseConfig (from Store via WithBaseConfig) or zero value
//  2. Then: configOpts applied in order (from WithLayerWatchConfig)
//  3. Finally: ApplyDefaults() fills any remaining zero/nil values
//
// The final ApplyDefaults() call ensures watcher implementations don't need
// defensive default handling - they can trust all config values are valid.
func (o *watchOptions) resolveConfig() watcher.WatchConfig {
	var cfg watcher.WatchConfig
	if o.baseConfig != nil {
		cfg = *o.baseConfig
	}
	cfg.ApplyOptions(o.configOpts...)
	cfg.ApplyDefaults()
	return cfg
}

// ResolveWatchConfig applies WatchOptions and returns the final merged WatchConfig.
// This is a convenience function for custom Layer implementations that want to use
// the same option merging logic as basicLayer.
//
// Example usage in a custom Layer.Watch() implementation:
//
//	func (l *myLayer) Watch(opts ...layer.WatchOption) (layer.LayerWatcher, error) {
//	    cfg := layer.ResolveWatchConfig(opts...)
//	    // Use cfg to create watcher...
//	}
func ResolveWatchConfig(opts ...WatchOption) watcher.WatchConfig {
	var options watchOptions
	for _, opt := range opts {
		opt(&options)
	}
	return options.resolveConfig()
}

// WithBaseConfig sets the base watch configuration.
// This is typically called by Store to pass store-level configuration.
// Layer-level options (via WithLayerWatchConfig) are applied after this.
func WithBaseConfig(cfg watcher.WatchConfig) WatchOption {
	return func(o *watchOptions) {
		o.baseConfig = &cfg
	}
}

// WithLayerWatchConfig allows layer-level override of WatchConfig.
// These options are applied after the base config (set via WithBaseConfig).
func WithLayerWatchConfig(opts ...watcher.WatchConfigOption) WatchOption {
	return func(o *watchOptions) {
		o.configOpts = append(o.configOpts, opts...)
	}
}
