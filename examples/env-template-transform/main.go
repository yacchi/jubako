// Example: env-template-transform
//
// This example demonstrates how to use text/template transformation in env tag placeholders.
// This feature allows you to transform environment variable keys (like "JP") into
// configuration path segments (like "jp") using standard template functions.
//
// Scenario:
// We have environment variables like:
//   APP_BACKLOG_CLIENT_ID_JP=client-id-jp
//   APP_BACKLOG_CLIENT_ID_US=client-id-us
//
// We want to map them to a map where keys are lowercased:
//   /backlog/jp/client_id
//   /backlog/us/client_id
//
// Without transformation, they would map to "JP" and "US", requiring
// case-sensitive handling in code or duplicate config entries.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/layer/env"
)

type BacklogConfig struct {
	ClientID string `json:"client_id" jubako:"env:BACKLOG_CLIENT_ID_{key|lower}"`
	Secret   string `json:"secret" jubako:"env:BACKLOG_SECRET_{key|lower}"`
}

type Config struct {
	// The map key will be populated from the captured {key} group,
	// transformed by the |lower filter.
	Backlog map[string]BacklogConfig `json:"backlog"`
}

func main() {
	// Set environment variables for demonstration
	os.Setenv("APP_BACKLOG_CLIENT_ID_JP", "client-id-japan")
	os.Setenv("APP_BACKLOG_SECRET_JP", "secret-japan")
	os.Setenv("APP_BACKLOG_CLIENT_ID_US", "client-id-usa")
	
	defer func() {
		os.Unsetenv("APP_BACKLOG_CLIENT_ID_JP")
		os.Unsetenv("APP_BACKLOG_SECRET_JP")
		os.Unsetenv("APP_BACKLOG_CLIENT_ID_US")
	}()

	// Create store
	store := jubako.New[Config]()

	// Add env layer with SchemaMapping
	// This enables the struct tag parsing and transformation logic.
	err := store.Add(
		env.New("env", "APP_", 
			env.WithSchemaMapping[Config](),
		),
		jubako.WithPriority(jubako.PriorityEnv),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Load configuration
	if err := store.Load(context.Background()); err != nil {
		log.Fatal(err)
	}

	// Get the configuration
	cfg := store.Get()

	fmt.Println("=== Environment Variable Transformation ===")
	fmt.Println("Environment Variables:")
	fmt.Println("  APP_BACKLOG_CLIENT_ID_JP = client-id-japan")
	fmt.Println("  APP_BACKLOG_CLIENT_ID_US = client-id-usa")
	fmt.Println()

	fmt.Println("Resulting Configuration (Map keys should be lowercase):")
	
	// Check Japan config (mapped from JP -> jp)
	if jpConfig, ok := cfg.Backlog["jp"]; ok {
		fmt.Printf("  backlog['jp'].ClientID: %s\n", jpConfig.ClientID)
		fmt.Printf("  backlog['jp'].Secret:   %s\n", jpConfig.Secret)
	} else {
		fmt.Println("  backlog['jp'] NOT FOUND (transformation failed?)")
	}

	// Check US config (mapped from US -> us, though US is simple)
	// Note: We didn't set SECRET for US, so it should be empty
	if usConfig, ok := cfg.Backlog["us"]; ok {
		fmt.Printf("  backlog['us'].ClientID: %s\n", usConfig.ClientID)
		fmt.Printf("  backlog['us'].Secret:   %s\n", usConfig.Secret)
	} else {
		// Currently the test code above didn't set US secret, so it's partially populated?
		// Actually jubako merges per field.
		// Wait, if I iterate the map it's better.
	}
	
	fmt.Println()
	fmt.Println("All Backlog Entries:")
	for key, val := range cfg.Backlog {
		fmt.Printf("  Key: %q\n", key)
		fmt.Printf("    ClientID: %s\n", val.ClientID)
		fmt.Printf("    Secret:   %s\n", val.Secret)
	}
}
