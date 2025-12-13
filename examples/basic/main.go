// Example: basic
//
// This example demonstrates basic usage of jubako for layered configuration.
// It shows how to:
// - Define a configuration struct
// - Create a store with multiple layers
// - Load and access configuration values
// - Modify and save configuration
//
// Run with: go run ./examples/basic
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/format/yaml"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source/bytes"
	"github.com/yacchi/jubako/source/fs"
)

// AppConfig defines the application configuration structure.
// Both yaml and json tags are required: yaml for parsing config files,
// json for internal materialization (merging layers into typed struct).
type AppConfig struct {
	Server   ServerConfig   `yaml:"server" json:"server"`
	Database DatabaseConfig `yaml:"database" json:"database"`
	Logging  LoggingConfig  `yaml:"logging" json:"logging"`
}

type ServerConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Name     string `yaml:"name" json:"name"`
	User     string `yaml:"user" json:"user"`
	Password string `yaml:"password" json:"password"`
}

type LoggingConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

// Default configuration embedded in the application
const defaultConfig = `
server:
  host: localhost
  port: 8080

database:
  host: localhost
  port: 5432
  name: myapp
  user: postgres
  password: ""

logging:
  level: info
  format: json
`

func main() {
	ctx := context.Background()

	// Create a store for AppConfig
	store := jubako.New[AppConfig]()

	// Add default configuration layer (lowest priority)
	err := store.Add(
		layer.New("defaults", bytes.FromString(defaultConfig), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityDefaults),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Create a temporary user config file for demonstration
	userConfigPath := createTempUserConfig()
	defer os.Remove(userConfigPath)

	// Add user configuration layer (higher priority)
	err = store.Add(
		layer.New("user", fs.New(userConfigPath), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityUser),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Load all layers
	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	// Get the merged configuration
	config := store.Get()

	fmt.Println("=== Merged Configuration ===")
	fmt.Printf("Server: %s:%d\n", config.Server.Host, config.Server.Port)
	fmt.Printf("Database: %s@%s:%d/%s\n",
		config.Database.User,
		config.Database.Host,
		config.Database.Port,
		config.Database.Name,
	)
	fmt.Printf("Logging: level=%s, format=%s\n", config.Logging.Level, config.Logging.Format)

	// Subscribe to configuration changes
	unsubscribe := store.Subscribe(func(cfg AppConfig) {
		fmt.Printf("\n[Notification] Config changed! New port: %d\n", cfg.Server.Port)
	})
	defer unsubscribe()

	// Modify configuration in user layer
	fmt.Println("\n=== Modifying user config ===")
	if err := store.SetTo("user", "/server/port", 9090); err != nil {
		log.Fatal(err)
	}

	// Get updated configuration
	config = store.Get()
	fmt.Printf("New server port: %d\n", config.Server.Port)

	// Save changes to user config file
	if err := store.Save(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("User config saved!")

	// Show saved content
	content, _ := os.ReadFile(userConfigPath)
	fmt.Printf("\n=== Saved user config file ===\n%s", content)
}

func createTempUserConfig() string {
	userConfig := `# User configuration
# This overrides default values

server:
  port: 8888  # Custom port

database:
  user: myuser
  password: secret123

logging:
  level: debug  # Enable debug logging
`
	tmpDir := os.TempDir()
	path := filepath.Join(tmpDir, "jubako-example-user.yaml")
	if err := os.WriteFile(path, []byte(userConfig), 0644); err != nil {
		log.Fatal(err)
	}
	return path
}
