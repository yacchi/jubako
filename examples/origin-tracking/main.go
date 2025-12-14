// Example: origin-tracking
//
// This example demonstrates jubako's origin tracking feature.
// It shows how to:
// - Track which layer each configuration value comes from
// - Use GetAt() to get a single value with its origin
// - Use Walk() with WalkContext.Value() to traverse all values with their origins
// - Use Walk() with WalkContext.AllValues() to see the full override chain
// - Debug configuration by understanding the effective source of each value
//
// Run with: go run ./examples/origin-tracking
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/format/yaml"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/layer/env"
	"github.com/yacchi/jubako/source/bytes"
)

type AppConfig struct {
	Server   ServerConfig   `yaml:"server" json:"server"`
	Database DatabaseConfig `yaml:"database" json:"database"`
	Cache    CacheConfig    `yaml:"cache" json:"cache"`
}

type ServerConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
}

type DatabaseConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
	Name string `yaml:"name" json:"name"`
}

type CacheConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Ttl     int    `yaml:"ttl" json:"ttl"`
	Backend string `yaml:"backend" json:"backend"`
}

const defaultConfig = `
server:
  host: localhost
  port: 8080

database:
  host: localhost
  port: 5432
  name: app_development

cache:
  enabled: false
  ttl: 300
  backend: memory
`

const userConfig = `
# User preferences
server:
  port: 9000

database:
  name: app_production

cache:
  enabled: true
  backend: redis
`

const projectConfig = `
# Project-specific settings
database:
  host: db.internal

cache:
  ttl: 600
`

func main() {
	ctx := context.Background()

	// Set environment variable for demonstration
	// Note: env layer only works with string fields due to type conversion
	os.Setenv("MYAPP_SERVER_HOST", "0.0.0.0")
	os.Setenv("MYAPP_CACHE_BACKEND", "memcached")
	defer func() {
		os.Unsetenv("MYAPP_SERVER_HOST")
		os.Unsetenv("MYAPP_CACHE_BACKEND")
	}()

	// Create store
	store := jubako.New[AppConfig]()

	// Add layers with different priorities
	store.Add(
		layer.New("defaults", bytes.FromString(defaultConfig), yaml.New()),
		jubako.WithPriority(jubako.PriorityDefaults),
	)

	store.Add(
		layer.New("user", bytes.FromString(userConfig), yaml.New()),
		jubako.WithPriority(jubako.PriorityUser),
	)

	store.Add(
		layer.New("project", bytes.FromString(projectConfig), yaml.New()),
		jubako.WithPriority(jubako.PriorityProject),
	)

	store.Add(
		env.New("env", "MYAPP_"),
		jubako.WithPriority(jubako.PriorityEnv),
	)

	// Load all layers
	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	// Show layer information
	fmt.Println("=== Registered Layers ===")
	for _, info := range store.ListLayers() {
		fmt.Printf("  [Priority %3d] %s\n", info.Priority(), info.Name())
	}
	fmt.Println()

	// Show merged configuration
	config := store.Get()
	fmt.Println("=== Merged Configuration ===")
	fmt.Printf("Server:   host=%s, port=%d\n", config.Server.Host, config.Server.Port)
	fmt.Printf("Database: host=%s, port=%d, name=%s\n",
		config.Database.Host, config.Database.Port, config.Database.Name)
	fmt.Printf("Cache:    enabled=%v, ttl=%d, backend=%s\n",
		config.Cache.Enabled, config.Cache.Ttl, config.Cache.Backend)
	fmt.Println()

	// Demonstrate GetAt() - get single value with origin
	fmt.Println("=== Origin Tracking with GetAt() ===")
	paths := []string{
		"/server/host",
		"/server/port",
		"/database/host",
		"/database/port",
		"/database/name",
		"/cache/enabled",
		"/cache/ttl",
		"/cache/backend",
	}

	for _, path := range paths {
		rv := store.GetAt(path)
		if rv.Exists {
			fmt.Printf("  %-20s = %-15v <- %s\n", path, rv.Value, rv.Layer.Name())
		} else {
			fmt.Printf("  %-20s = (not set)\n", path)
		}
	}
	fmt.Println()

	// Demonstrate Walk() - traverse all values with effective origin
	fmt.Println("=== Full Configuration Walk with Origins ===")
	store.Walk(func(ctx jubako.WalkContext) bool {
		rv := ctx.Value()
		layerName := "(unknown)"
		if rv.Layer != nil {
			layerName = string(rv.Layer.Name())
		}
		fmt.Printf("  %-25s = %-15v <- %s\n", ctx.Path, rv.Value, layerName)
		return true // continue walking
	})
	fmt.Println()

	// Demonstrate Walk() with AllValues() - show override chain for paths with multiple values
	fmt.Println("=== Override Analysis (paths with multiple layers) ===")
	store.Walk(func(ctx jubako.WalkContext) bool {
		allValues := ctx.AllValues()

		// Only show paths that have values from multiple layers
		if allValues.Len() <= 1 {
			return true
		}

		fmt.Printf("%s:\n", ctx.Path)

		// Show each layer's value (sorted by priority, lowest first)
		for _, rv := range allValues {
			fmt.Printf("  - %-10s: %v\n", rv.Layer.Name(), rv.Value)
		}

		// Show the effective value using Effective()
		effective := allValues.Effective()
		fmt.Printf("  -> Result: %v (from %s)\n", effective.Value, effective.Layer.Name())
		fmt.Println()

		return true
	})
}
