// Example: transform-layer
//
// This example demonstrates how to create a layer wrapper that transforms
// paths bidirectionally. This pattern is useful for:
// - Supporting multiple configuration schema versions (v1/v2)
// - Migrating between different key naming conventions
// - Providing backwards compatibility during refactoring
//
// The TransformLayer wraps an existing layer and transforms data for
// Load and Save operations, allowing code to use a canonical path
// format while the underlying document uses a different structure.
//
// Run with: go run ./examples/transform-layer
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/format/yaml"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source/bytes"
)

func main() {
	fmt.Println("=== Transform Layer Example ===")
	fmt.Println()
	fmt.Println("This example shows how to support multiple config schema versions.")
	fmt.Println("The application uses v2 (canonical) paths, but can read v1 (legacy) files.")
	fmt.Println()

	// Example 1: Using v2 config directly (no transformation needed)
	fmt.Println("--- Example 1: v2 Config (native format) ---")
	runWithConfig("v2", v2Config, nil)

	fmt.Println()

	// Example 2: Using v1 config with transformation layer
	fmt.Println("--- Example 2: v1 Config (with transformation) ---")
	runWithConfig("v1", v1Config, v1ToV2Mappings)

	fmt.Println()

	// Example 3: Demonstrate transformation in action
	fmt.Println("--- Example 3: Transformation Details ---")
	demonstrateTransformation()
}

func runWithConfig(name, configData string, mappings []PathMapping) {
	ctx := context.Background()
	store := jubako.New[AppConfig]()

	// Create the base layer
	baseLayer := layer.New(layer.Name(name), bytes.FromString(configData), yaml.New())

	// Wrap with transform layer if mappings are provided
	var layerToAdd layer.Layer = baseLayer
	if mappings != nil {
		layerToAdd = NewTransformLayer(baseLayer, mappings)
	}

	if err := store.Add(layerToAdd); err != nil {
		log.Fatal(err)
	}

	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	config := store.Get()

	fmt.Printf("Database: %s@%s:%d/%s\n",
		config.Database.User,
		config.Database.Host,
		config.Database.Port,
		config.Database.Name,
	)
	fmt.Printf("Server: %s:%d\n", config.Server.Host, config.Server.Port)

	// Show origin tracking still works
	rv := store.GetAt("/database/host")
	if rv.Exists {
		fmt.Printf("(database.host from layer: %s)\n", rv.Layer.Name())
	}
}

func demonstrateTransformation() {
	ctx := context.Background()

	// Create v1 layer with transformation
	baseLayer := layer.New("v1-config", bytes.FromString(v1Config), yaml.New())
	transformLayer := NewTransformLayer(baseLayer, v1ToV2Mappings)

	// Load the layer - data is automatically transformed
	data, err := transformLayer.Load(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Show that data uses canonical (v2) paths
	fmt.Println("Loaded data (transformed to canonical format):")
	if database, ok := data["database"].(map[string]any); ok {
		fmt.Printf("  database.host = %v\n", database["host"])
		fmt.Printf("  database.port = %v\n", database["port"])
		fmt.Printf("  database.name = %v\n", database["name"])
		fmt.Printf("  database.user = %v\n", database["user"])
	}
	if server, ok := data["server"].(map[string]any); ok {
		fmt.Printf("  server.host = %v\n", server["host"])
		fmt.Printf("  server.port = %v\n", server["port"])
	}

	// Show path mappings used
	fmt.Println("\nPath mappings (v1 -> v2):")
	for _, m := range v1ToV2Mappings {
		fmt.Printf("  %s -> %s\n", m.Source, m.Canonical)
	}
}
