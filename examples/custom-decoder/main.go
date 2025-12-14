// Example: Custom Decoder with mapstructure
//
// This example demonstrates how to use a custom decoder (mapstructure)
// instead of the default JSON-based decoder.
//
// Benefits of mapstructure:
// - Use `mapstructure` tags instead of `json` tags
// - Weak type conversion (strings to numbers, etc.)
// - Embedded struct support
// - Remaining fields capture
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mitchellh/mapstructure"
	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/layer/mapdata"
)

// =============================================================================
// Configuration Types (using mapstructure tags)
// =============================================================================

// Config uses mapstructure tags instead of json tags
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Features []string       `mapstructure:"features"`
}

type ServerConfig struct {
	Host    string `mapstructure:"host"`
	Port    int    `mapstructure:"port"`
	Timeout int    `mapstructure:"timeout"`
}

type DatabaseConfig struct {
	URL         string `mapstructure:"url"`
	MaxPoolSize int    `mapstructure:"max_pool_size"`
}

// =============================================================================
// Custom Decoder Implementation
// =============================================================================

// mapstructureDecoder wraps mapstructure.Decode to match decoder.Func signature.
// The decoder.Func signature is:
//
//	func(data map[string]any, target any) error
func mapstructureDecoder(data map[string]any, target any) error {
	config := &mapstructure.DecoderConfig{
		Result:           target,
		WeaklyTypedInput: true, // Enable weak type conversion (e.g., string "8080" to int 8080)
		TagName:          "mapstructure",
	}
	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}
	return decoder.Decode(data)
}

func main() {
	ctx := context.Background()

	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║     Custom Decoder Example (mapstructure)                      ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")

	// =============================================================================
	// Example 1: Basic usage with weak type conversion
	// =============================================================================

	fmt.Println("\n=== Example 1: Weak Type Conversion ===")

	// Note: port and timeout are strings, but mapstructure will convert them to int
	configData := map[string]any{
		"server": map[string]any{
			"host":    "localhost",
			"port":    "8080",  // String that will be converted to int
			"timeout": "30",    // String that will be converted to int
		},
		"database": map[string]any{
			"url":           "postgres://localhost/myapp",
			"max_pool_size": 10,
		},
		"features": []any{"feature1", "feature2", "feature3"},
	}

	// Create store with custom decoder
	store := jubako.New[Config](jubako.WithDecoder(mapstructureDecoder))

	if err := store.Add(mapdata.New("config", configData)); err != nil {
		log.Fatal(err)
	}
	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	config := store.Get()

	fmt.Println("\nResolved Configuration:")
	fmt.Printf("  Server.Host:           %s\n", config.Server.Host)
	fmt.Printf("  Server.Port:           %d (converted from string)\n", config.Server.Port)
	fmt.Printf("  Server.Timeout:        %d (converted from string)\n", config.Server.Timeout)
	fmt.Printf("  Database.URL:          %s\n", config.Database.URL)
	fmt.Printf("  Database.MaxPoolSize:  %d\n", config.Database.MaxPoolSize)
	fmt.Printf("  Features:              %v\n", config.Features)

	// =============================================================================
	// Example 2: Layer override with type conversion
	// =============================================================================

	fmt.Println("\n=== Example 2: Layer Override with Type Conversion ===")

	store2 := jubako.New[Config](jubako.WithDecoder(mapstructureDecoder))

	// Base layer with defaults
	baseConfig := map[string]any{
		"server": map[string]any{
			"host":    "0.0.0.0",
			"port":    8080,
			"timeout": 60,
		},
		"database": map[string]any{
			"url":           "postgres://localhost/default",
			"max_pool_size": 5,
		},
		"features": []any{"default-feature"},
	}

	// Override layer (simulating environment variable overrides)
	// Note: Environment variables are typically strings
	overrideConfig := map[string]any{
		"server": map[string]any{
			"port":    "9000", // String override (like from env var)
			"timeout": "120",  // String override
		},
		"database": map[string]any{
			"max_pool_size": "20", // String override
		},
	}

	if err := store2.Add(
		mapdata.New("base", baseConfig),
		jubako.WithPriority(jubako.PriorityDefaults),
	); err != nil {
		log.Fatal(err)
	}

	if err := store2.Add(
		mapdata.New("override", overrideConfig),
		jubako.WithPriority(jubako.PriorityEnv),
	); err != nil {
		log.Fatal(err)
	}

	if err := store2.Load(ctx); err != nil {
		log.Fatal(err)
	}

	config2 := store2.Get()

	fmt.Println("\nResolved Configuration (with overrides):")
	fmt.Printf("  Server.Host:           %s (from base)\n", config2.Server.Host)
	fmt.Printf("  Server.Port:           %d (overridden, converted from string)\n", config2.Server.Port)
	fmt.Printf("  Server.Timeout:        %d (overridden, converted from string)\n", config2.Server.Timeout)
	fmt.Printf("  Database.URL:          %s (from base)\n", config2.Database.URL)
	fmt.Printf("  Database.MaxPoolSize:  %d (overridden, converted from string)\n", config2.Database.MaxPoolSize)
	fmt.Printf("  Features:              %v (from base)\n", config2.Features)

	// Show origin tracking
	fmt.Println("\nOrigin Tracking:")
	rv := store2.GetAt("/server/port")
	if rv.Exists {
		fmt.Printf("  /server/port = %v (from layer: %s)\n", rv.Value, rv.Layer.Name())
	}
	rv = store2.GetAt("/server/host")
	if rv.Exists {
		fmt.Printf("  /server/host = %v (from layer: %s)\n", rv.Value, rv.Layer.Name())
	}
}
