// Example: env-override
//
// This example demonstrates how to use environment variables to override
// configuration values. It shows:
// - Using the env layer to read from environment variables
// - Priority ordering (env > file > defaults)
// - Environment variable naming convention (prefix + underscore-separated path)
//
// Note: Environment variables are always read as strings. For type conversion,
// the configuration struct fields should be string type, or you should implement
// custom conversion logic.
//
// The env layer converts variable names like this:
//
//	APP_SERVER_HOST -> /server/host
//	APP_DATABASE_USER -> /database/user
//
// Run with:
//
//	APP_SERVER_HOST=0.0.0.0 APP_DATABASE_HOST=prod-db.example.com go run ./examples/env-override
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

// Note: For environment variable overrides, use string types.
// The env layer always reads values as strings.
type AppConfig struct {
	Server   ServerConfig   `yaml:"server" json:"server"`
	Database DatabaseConfig `yaml:"database" json:"database"`
	Feature  FeatureFlags   `yaml:"feature" json:"feature"`
}

type ServerConfig struct {
	Host    string `yaml:"host" json:"host"`
	Port    int    `yaml:"port" json:"port"`       // Set via YAML only
	Timeout int    `yaml:"timeout" json:"timeout"` // Set via YAML only
}

type DatabaseConfig struct {
	Host     string `yaml:"host" json:"host"`         // Can be overridden by env
	Port     int    `yaml:"port" json:"port"`         // Set via YAML only
	Name     string `yaml:"name" json:"name"`         // Can be overridden by env
	User     string `yaml:"user" json:"user"`         // Can be overridden by env
	Password string `yaml:"password" json:"password"` // Can be overridden by env
}

type FeatureFlags struct {
	Backend  string `yaml:"backend" json:"backend"`   // Can be overridden by env (APP_FEATURE_BACKEND)
	Loglevel string `yaml:"loglevel" json:"loglevel"` // Can be overridden by env (APP_FEATURE_LOGLEVEL)
}

const defaultConfig = `
server:
  host: localhost
  port: 8080
  timeout: 30

database:
  host: localhost
  port: 5432
  name: myapp
  user: postgres
  password: ""

feature:
  backend: memory
  loglevel: info
`

func main() {
	ctx := context.Background()

	// Set some environment variables for demonstration
	// In real usage, these would be set externally
	os.Setenv("APP_SERVER_HOST", "0.0.0.0")
	os.Setenv("APP_DATABASE_HOST", "prod-db.example.com")
	os.Setenv("APP_DATABASE_NAME", "production_db")
	os.Setenv("APP_DATABASE_USER", "prod_user")
	os.Setenv("APP_DATABASE_PASSWORD", "super-secret")
	os.Setenv("APP_FEATURE_LOGLEVEL", "debug")
	os.Setenv("APP_FEATURE_BACKEND", "redis")
	defer func() {
		os.Unsetenv("APP_SERVER_HOST")
		os.Unsetenv("APP_DATABASE_HOST")
		os.Unsetenv("APP_DATABASE_NAME")
		os.Unsetenv("APP_DATABASE_USER")
		os.Unsetenv("APP_DATABASE_PASSWORD")
		os.Unsetenv("APP_FEATURE_LOGLEVEL")
		os.Unsetenv("APP_FEATURE_BACKEND")
	}()

	// Create store
	store := jubako.New[AppConfig]()

	// Layer 1: Default values (lowest priority)
	err := store.Add(
		layer.New("defaults", bytes.FromString(defaultConfig), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityDefaults),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Layer 2: Environment variables (highest priority)
	// Variables with prefix "APP_" are included
	// APP_SERVER_HOST -> /server/host
	// APP_DATABASE_HOST -> /database/host
	err = store.Add(
		env.New("env", "APP_"),
		jubako.WithPriority(jubako.PriorityEnv),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Load all layers
	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	// Get merged configuration
	config := store.Get()

	fmt.Println("=== Configuration with Environment Overrides ===")
	fmt.Println()

	fmt.Println("Server:")
	fmt.Printf("  Host: %s (env override from APP_SERVER_HOST)\n", config.Server.Host)
	fmt.Printf("  Port: %d (default - int types need YAML layer)\n", config.Server.Port)
	fmt.Printf("  Timeout: %d (default)\n", config.Server.Timeout)
	fmt.Println()

	fmt.Println("Database:")
	fmt.Printf("  Host: %s (env override from APP_DATABASE_HOST)\n", config.Database.Host)
	fmt.Printf("  Port: %d (default - int types need YAML layer)\n", config.Database.Port)
	fmt.Printf("  Name: %s (env override from APP_DATABASE_NAME)\n", config.Database.Name)
	fmt.Printf("  User: %s (env override from APP_DATABASE_USER)\n", config.Database.User)
	fmt.Printf("  Password: %s (env override, masked)\n", maskPassword(config.Database.Password))
	fmt.Println()

	fmt.Println("Feature Flags:")
	fmt.Printf("  Backend: %s (env override from APP_FEATURE_BACKEND)\n", config.Feature.Backend)
	fmt.Printf("  Loglevel: %s (env override from APP_FEATURE_LOGLEVEL)\n", config.Feature.Loglevel)
	fmt.Println()

	// Show layer information
	fmt.Println("=== Registered Layers (by priority) ===")
	for _, info := range store.ListLayers() {
		fmt.Printf("  [%d] %s (format: %s, writable: %v)\n",
			info.Priority(),
			info.Name(),
			info.Format(),
			info.Writable(),
		)
	}
	fmt.Println()

	// Show origin tracking
	fmt.Println("=== Origin Tracking ===")
	paths := []string{
		"/server/host",
		"/server/port",
		"/database/host",
		"/database/user",
		"/feature/loglevel",
		"/feature/backend",
	}
	for _, path := range paths {
		rv := store.GetAt(path)
		if rv.Exists {
			fmt.Printf("  %-25s = %-20v <- layer: %s\n", path, rv.Value, rv.Layer.Name())
		}
	}
}

func maskPassword(p string) string {
	if len(p) == 0 {
		return "(empty)"
	}
	return "****"
}
