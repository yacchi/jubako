// Example: Path Remapping with jubako struct tags
//
// This example demonstrates how to use jubako tags to remap nested YAML/TOML/JSON
// paths to flat struct fields. This is useful when:
// - Config files are nested for human readability
// - Application code prefers flat structs for convenience
//
// Path types:
// - Absolute path: "/server/host" - resolved from root
// - Relative path: "connection/host" - resolved from current context (useful in slices/maps)
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/layer/mapdata"
)

// =============================================================================
// Example 1: Absolute paths (flat struct from nested config)
// =============================================================================

// ServerConfig demonstrates path remapping using absolute paths.
// The jubako tag specifies the source path from the root of the config.
//
// YAML structure (nested):
//
//	server:
//	  host: "0.0.0.0"
//	  port: 8080
//	  http:
//	    read_timeout: 10
//	    write_timeout: 30
//
// Go struct (flat):
type ServerConfig struct {
	Host             string `json:"host" jubako:"/server/host"`
	Port             int    `json:"port" jubako:"/server/port"`
	HTTPReadTimeout  int    `json:"http_read_timeout" jubako:"/server/http/read_timeout"`
	HTTPWriteTimeout int    `json:"http_write_timeout" jubako:"/server/http/write_timeout"`
	// Fields with jubako:"-" are skipped during remapping
	InternalField string `json:"internal" jubako:"-"`
}

// =============================================================================
// Example 2: Relative paths in slice elements
// =============================================================================

// Node demonstrates relative path remapping within slice elements.
// Relative paths (no leading "/") are resolved from each element's context.
//
// YAML structure:
//
//	defaults:
//	  timeout: 30
//	nodes:
//	  - connection:
//	      host: node1.example.com
//	      port: 5432
//	  - connection:
//	      host: node2.example.com
//	      port: 5433
type Node struct {
	// Relative paths - resolved from each slice element
	Host string `json:"host" jubako:"connection/host"`
	Port int    `json:"port" jubako:"connection/port"`
	// Absolute path - resolved from root (shared default for all nodes)
	DefaultTimeout int `json:"default_timeout" jubako:"/defaults/timeout"`
}

type ClusterConfig struct {
	Nodes []Node `json:"nodes"`
}

// =============================================================================
// Example 3: Relative paths in map values
// =============================================================================

// ServiceConfig demonstrates relative path remapping within map values.
//
// YAML structure:
//
//	defaults:
//	  retries: 3
//	services:
//	  api:
//	    settings:
//	      endpoint: https://api.example.com
//	  web:
//	    settings:
//	      endpoint: https://web.example.com
type ServiceConfig struct {
	// Relative path - resolved from each map value
	Endpoint string `json:"endpoint" jubako:"settings/endpoint"`
	// Absolute path - resolved from root
	DefaultRetries int `json:"default_retries" jubako:"/defaults/retries"`
}

type AppConfig struct {
	Services map[string]ServiceConfig `json:"services"`
}

func main() {
	ctx := context.Background()

	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║     Path Remapping Examples with jubako struct tags            ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")

	example1AbsolutePaths(ctx)
	example2SliceRelativePaths(ctx)
	example3MapRelativePaths(ctx)
}

func example1AbsolutePaths(ctx context.Context) {
	fmt.Println("\n=== Example 1: Absolute Paths ===")

	configData := map[string]any{
		"server": map[string]any{
			"host": "0.0.0.0",
			"port": 8080,
			"http": map[string]any{
				"read_timeout":  10,
				"write_timeout": 30,
			},
		},
		"internal": "should be ignored",
	}

	store := jubako.New[ServerConfig]()

	fmt.Println("\nMapping Table:")
	fmt.Println(store.MappingTable())

	if err := store.Add(mapdata.New("config", configData)); err != nil {
		log.Fatal(err)
	}
	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	config := store.Get()

	fmt.Println("Resolved Configuration:")
	fmt.Printf("  Host:             %s\n", config.Host)
	fmt.Printf("  Port:             %d\n", config.Port)
	fmt.Printf("  HTTPReadTimeout:  %d\n", config.HTTPReadTimeout)
	fmt.Printf("  HTTPWriteTimeout: %d\n", config.HTTPWriteTimeout)
	fmt.Printf("  InternalField:    %q (skipped)\n", config.InternalField)
}

func example2SliceRelativePaths(ctx context.Context) {
	fmt.Println("\n=== Example 2: Relative Paths in Slice Elements ===")

	configData := map[string]any{
		"defaults": map[string]any{
			"timeout": 30,
		},
		"nodes": []any{
			map[string]any{
				"connection": map[string]any{
					"host": "node1.example.com",
					"port": 5432,
				},
			},
			map[string]any{
				"connection": map[string]any{
					"host": "node2.example.com",
					"port": 5433,
				},
			},
		},
	}

	store := jubako.New[ClusterConfig]()

	fmt.Println("\nMapping Table:")
	fmt.Println(store.MappingTable())

	if err := store.Add(mapdata.New("config", configData)); err != nil {
		log.Fatal(err)
	}
	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	config := store.Get()

	fmt.Println("Resolved Configuration:")
	for i, node := range config.Nodes {
		fmt.Printf("  Node[%d]:\n", i)
		fmt.Printf("    Host:           %s (from relative path)\n", node.Host)
		fmt.Printf("    Port:           %d (from relative path)\n", node.Port)
		fmt.Printf("    DefaultTimeout: %d (from absolute path /defaults/timeout)\n", node.DefaultTimeout)
	}
}

func example3MapRelativePaths(ctx context.Context) {
	fmt.Println("\n=== Example 3: Relative Paths in Map Values ===")

	configData := map[string]any{
		"defaults": map[string]any{
			"retries": 3,
		},
		"services": map[string]any{
			"api": map[string]any{
				"settings": map[string]any{
					"endpoint": "https://api.example.com",
				},
			},
			"web": map[string]any{
				"settings": map[string]any{
					"endpoint": "https://web.example.com",
				},
			},
		},
	}

	store := jubako.New[AppConfig]()

	fmt.Println("\nMapping Table:")
	fmt.Println(store.MappingTable())

	if err := store.Add(mapdata.New("config", configData)); err != nil {
		log.Fatal(err)
	}
	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	config := store.Get()

	fmt.Println("Resolved Configuration:")
	for name, svc := range config.Services {
		fmt.Printf("  Service[%s]:\n", name)
		fmt.Printf("    Endpoint:       %s (from relative path)\n", svc.Endpoint)
		fmt.Printf("    DefaultRetries: %d (from absolute path /defaults/retries)\n", svc.DefaultRetries)
	}
}
