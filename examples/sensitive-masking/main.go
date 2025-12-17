// Example: sensitive-masking
//
// This example demonstrates jubako's sensitive data handling features:
// - Marking fields as sensitive with struct tags (`jubako:"sensitive"`)
// - Marking layers as sensitive with WithSensitive() option
// - Automatic masking of sensitive values in GetAt and Walk
// - Prevention of cross-contamination between sensitive and normal layers
//
// Key concepts:
// - Field sensitivity: Controlled by struct tags, determines WHAT data is sensitive
// - Layer sensitivity: Controlled by WithSensitive(), determines WHERE sensitive data can be stored
//
// Run with: go run ./examples/sensitive-masking
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/layer/mapdata"
)

// AppConfig demonstrates sensitive field marking with struct tags.
//
// Path resolution follows the same convention as encoding/json:
// - If a json tag exists, its key is used (e.g., `json:"name"` -> "/app/name")
// - Otherwise, the struct field name is used (e.g., `Name` -> "/app/Name")
//
// Sensitivity rules:
// - `jubako:"sensitive"` on a struct field: All nested fields inherit sensitivity
// - `jubako:"sensitive"` on a leaf field: Only that field is sensitive
// - `jubako:"!sensitive"`: Opts out of inherited sensitivity
type AppConfig struct {
	// App is a normal (non-sensitive) struct
	App AppSettings `json:"app"`

	// Credentials is marked sensitive - ALL nested fields inherit sensitivity
	// (except those explicitly opted out with !sensitive)
	Credentials Credentials `json:"credentials" jubako:"sensitive"`

	// Database is a normal struct, but individual fields can be marked sensitive
	Database DBConfig `json:"database"`
}

type AppSettings struct {
	Name    string `json:"name"`    // Normal field
	Version string `json:"version"` // Normal field
}

type Credentials struct {
	// These fields inherit sensitivity from parent struct
	APIKey    string `json:"api_key"`    // Sensitive (inherited)
	SecretKey string `json:"secret_key"` // Sensitive (inherited)

	// PublicKey opts out of inherited sensitivity
	PublicKey string `json:"public_key" jubako:"!sensitive"` // NOT sensitive (opt-out)
}

type DBConfig struct {
	Host string `json:"host"` // Normal field
	Port int    `json:"port"` // Normal field

	// Password is individually marked sensitive within a non-sensitive struct
	Password string `json:"password" jubako:"sensitive"` // Sensitive (explicit)
}

func main() {
	ctx := context.Background()

	fmt.Println("=== Jubako Sensitive Data Handling Demo ===")
	fmt.Println()

	// Part 1: Demonstrate field sensitivity and masking
	demonstrateMasking(ctx)

	// Part 2: Demonstrate write protection between layers
	demonstrateWriteProtection(ctx)
}

func demonstrateMasking(ctx context.Context) {
	fmt.Println("--- Part 1: Sensitive Value Masking ---")
	fmt.Println()
	fmt.Println("Field sensitivity is determined by struct tags:")
	fmt.Println("  - `jubako:\"sensitive\"` marks a field (and its children) as sensitive")
	fmt.Println("  - `jubako:\"!sensitive\"` opts out of inherited sensitivity")
	fmt.Println()

	// Create store with sensitive masking enabled
	store := jubako.New[AppConfig](
		jubako.WithSensitiveMaskString("[REDACTED]"),
	)

	// Add a sensitive layer with all the data
	// Note: Layer sensitivity (WithSensitive) controls WHERE data can be written,
	// while field sensitivity (struct tags) controls WHAT gets masked
	if err := store.Add(
		mapdata.New("config", map[string]any{
			"app": map[string]any{
				"name":    "myapp",
				"version": "1.0.0",
			},
			"credentials": map[string]any{
				"api_key":    "sk-prod-abc123xyz789",
				"secret_key": "super-secret-key-12345",
				"public_key": "pk-public-abc123",
			},
			"database": map[string]any{
				"host":     "localhost",
				"port":     5432,
				"password": "db-password-456",
			},
		}),
		jubako.WithSensitive(),
	); err != nil {
		log.Fatal(err)
	}

	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	// Show all fields with their sensitivity status
	fmt.Println("All configuration values (GetAt with automatic masking):")
	fmt.Println()
	fmt.Printf("  %-30s %-25s %s\n", "Path", "Value", "Status")
	fmt.Printf("  %-30s %-25s %s\n", "----", "-----", "------")

	paths := []struct {
		path        string
		description string
	}{
		{"/app/name", "normal field"},
		{"/app/version", "normal field"},
		{"/credentials/api_key", "sensitive (inherited)"},
		{"/credentials/secret_key", "sensitive (inherited)"},
		{"/credentials/public_key", "!sensitive (opt-out)"},
		{"/database/host", "normal field"},
		{"/database/port", "normal field"},
		{"/database/password", "sensitive (explicit)"},
	}

	for _, p := range paths {
		rv := store.GetAt(p.path)
		status := p.description
		if rv.Masked {
			status += " [MASKED]"
		}
		fmt.Printf("  %-30s %-25v %s\n", p.path, rv.Value, status)
	}

	// Demonstrate GetAtUnmasked
	fmt.Println()
	fmt.Println("GetAtUnmasked returns the actual value (use carefully!):")
	realApiKey := store.GetAtUnmasked("/credentials/api_key")
	fmt.Printf("  /credentials/api_key = %v\n", realApiKey.Value)

	// Walk demonstration
	fmt.Println()
	fmt.Println("Walk provides IsSensitive() for each path:")
	fmt.Println()
	store.Walk(func(ctx jubako.WalkContext) bool {
		rv := ctx.Value()
		// Skip container paths (maps/objects)
		if rv.Value == nil {
			return true
		}
		if _, ok := rv.Value.(map[string]any); ok {
			return true
		}

		marker := ""
		if ctx.IsSensitive() {
			marker = " <- sensitive"
		}
		fmt.Printf("  %-30s = %-20v%s\n", ctx.Path, rv.Value, marker)
		return true
	})
	fmt.Println()
}

func demonstrateWriteProtection(ctx context.Context) {
	fmt.Println("--- Part 2: Write Protection ---")
	fmt.Println()
	fmt.Println("Layer sensitivity controls WHERE sensitive data can be stored:")
	fmt.Println("  - Sensitive fields can ONLY be written to sensitive layers")
	fmt.Println("  - Normal fields can ONLY be written to normal layers")
	fmt.Println("  - This prevents accidental cross-contamination")
	fmt.Println()

	store := jubako.New[AppConfig]()

	// Add a normal layer (e.g., project config file)
	if err := store.Add(
		mapdata.New("project", map[string]any{
			"app": map[string]any{"name": "test"},
		}),
		jubako.WithPriority(0),
	); err != nil {
		log.Fatal(err)
	}

	// Add a sensitive layer (e.g., secrets file)
	if err := store.Add(
		mapdata.New("secrets", map[string]any{
			"credentials": map[string]any{"api_key": "old-key"},
		}),
		jubako.WithPriority(10),
		jubako.WithSensitive(),
	); err != nil {
		log.Fatal(err)
	}

	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	// Show layer configuration
	fmt.Println("Layer configuration:")
	for _, info := range store.ListLayers() {
		layerType := "normal"
		if info.Sensitive() {
			layerType = "sensitive"
		}
		fmt.Printf("  - %s (priority: %d, type: %s)\n",
			info.Name(), info.Priority(), layerType)
	}
	fmt.Println()

	// Test cases
	fmt.Println("Write operation tests:")
	fmt.Println()

	// Case 1: Sensitive field -> Normal layer (ERROR)
	fmt.Println("1. SetTo(\"project\", \"/credentials/api_key\", \"new-secret\")")
	fmt.Println("   Field: sensitive (inherited from credentials)")
	fmt.Println("   Layer: normal")
	err := store.SetTo("project", "/credentials/api_key", "new-secret")
	if err != nil {
		fmt.Printf("   Result: ERROR - %v\n", err)
	}

	// Case 2: Normal field -> Sensitive layer (ERROR)
	fmt.Println()
	fmt.Println("2. SetTo(\"secrets\", \"/app/name\", \"myapp\")")
	fmt.Println("   Field: normal")
	fmt.Println("   Layer: sensitive")
	err = store.SetTo("secrets", "/app/name", "myapp")
	if err != nil {
		fmt.Printf("   Result: ERROR - %v\n", err)
	}

	// Case 3: Sensitive field -> Sensitive layer (OK)
	fmt.Println()
	fmt.Println("3. SetTo(\"secrets\", \"/credentials/api_key\", \"new-api-key\")")
	fmt.Println("   Field: sensitive (inherited)")
	fmt.Println("   Layer: sensitive")
	err = store.SetTo("secrets", "/credentials/api_key", "new-api-key")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("   Result: SUCCESS")

	// Case 4: Normal field -> Normal layer (OK)
	fmt.Println()
	fmt.Println("4. SetTo(\"project\", \"/app/name\", \"updated-app\")")
	fmt.Println("   Field: normal")
	fmt.Println("   Layer: normal")
	err = store.SetTo("project", "/app/name", "updated-app")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("   Result: SUCCESS")

	// Case 5: !sensitive field -> Normal layer (OK, because it opted out)
	fmt.Println()
	fmt.Println("5. SetTo(\"project\", \"/credentials/public_key\", \"new-public-key\")")
	fmt.Println("   Field: !sensitive (opted out of inheritance)")
	fmt.Println("   Layer: normal")
	err = store.SetTo("project", "/credentials/public_key", "new-public-key")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("   Result: SUCCESS")
}
