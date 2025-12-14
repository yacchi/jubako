// Example: transform-layer
//
// This example demonstrates how to create a layer wrapper that transforms
// paths bidirectionally. This pattern is useful for:
// - Supporting multiple configuration schema versions (v1/v2)
// - Migrating between different key naming conventions
// - Providing backwards compatibility during refactoring
//
// The TransformLayer wraps an existing layer and transforms paths for
// Get, Set, and Delete operations, allowing code to use a canonical path
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

	// Example 3: Demonstrate direct document access with transformation
	fmt.Println("--- Example 3: Direct Document Access ---")
	demonstrateDirectAccess()
}

func runWithConfig(name, configData string, mappings []PathMapping) {
	ctx := context.Background()
	store := jubako.New[AppConfig]()

	// Create the base layer
	baseLayer := layer.New(layer.Name(name), bytes.FromString(configData), yaml.NewParser())

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

func demonstrateDirectAccess() {
	ctx := context.Background()

	// Create v1 layer with transformation
	baseLayer := layer.New("v1-config", bytes.FromString(v1Config), yaml.NewParser())
	transformLayer := NewTransformLayer(baseLayer, v1ToV2Mappings)

	// Load the layer
	doc, err := transformLayer.Load(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Access using canonical (v2) paths
	fmt.Println("Reading with canonical paths:")
	if val, ok := doc.Get("/database/host"); ok {
		fmt.Printf("  /database/host = %v\n", val)
	}
	if val, ok := doc.Get("/database/port"); ok {
		fmt.Printf("  /database/port = %v\n", val)
	}
	if val, ok := doc.Get("/server/host"); ok {
		fmt.Printf("  /server/host = %v\n", val)
	}

	// Write using canonical path (writes to v1 structure)
	fmt.Println("\nModifying /database/port to 5433...")
	if err := doc.Set("/database/port", 5433); err != nil {
		log.Fatal(err)
	}

	// Verify the change
	if val, ok := doc.Get("/database/port"); ok {
		fmt.Printf("  /database/port = %v (after modification)\n", val)
	}

	// Show that the underlying v1 structure was modified
	innerDoc := baseLayer.Document()
	if val, ok := innerDoc.Get("/db/port"); ok {
		fmt.Printf("  Underlying /db/port = %v\n", val)
	}

	// Marshal to show the v1 format is preserved
	data, err := doc.Marshal()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\nSerialized v1 config (comments preserved):")
	fmt.Println(string(data))
}
