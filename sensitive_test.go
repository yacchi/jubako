package jubako

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/yacchi/jubako/layer/mapdata"
)

// Test configuration types for sensitive layer tests
// Note: sensitive tags should ONLY be applied to leaf fields (string, int, etc.)
// Applying sensitive to container types (struct, map, slice) will trigger a warning.
type sensitiveTestConfig struct {
	App         appSettings    `json:"app"`
	Credentials credentialData `json:"credentials"`
	Database    databaseConfig `json:"database"`
}

type appSettings struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type credentialData struct {
	APIKey    string `json:"api_key" jubako:"sensitive"`  // Explicitly sensitive
	Password  string `json:"password" jubako:"sensitive"` // Explicitly sensitive
	PublicKey string `json:"public_key"`                  // Not sensitive
}

type databaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password" jubako:"sensitive"` // Explicitly sensitive
}

func TestSensitiveLayerValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("sensitive field to normal layer returns error", func(t *testing.T) {
		store := New[sensitiveTestConfig]()

		// Add a normal layer
		if err := store.Add(mapdata.New("normal", map[string]any{
			"app": map[string]any{"name": "test"},
		})); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		// Try to write sensitive field to normal layer - should fail
		err := store.SetTo("normal", "/credentials/api_key", "secret123")
		if err == nil {
			t.Fatal("expected error when writing sensitive field to normal layer")
		}
		if !errors.Is(err, ErrSensitiveFieldToNormalLayer) {
			t.Fatalf("expected ErrSensitiveFieldToNormalLayer, got: %v", err)
		}
	})

	t.Run("normal field to sensitive layer succeeds", func(t *testing.T) {
		store := New[sensitiveTestConfig]()

		// Add a sensitive layer
		if err := store.Add(mapdata.New("secrets", map[string]any{
			"credentials": map[string]any{"api_key": "test"},
		}), WithSensitive()); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		// Write normal field to sensitive layer - should succeed
		// This allows storing related non-sensitive data (e.g., account IDs)
		// alongside sensitive data in secure storage locations.
		err := store.SetTo("secrets", "/app/name", "myapp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the value was set
		config := store.Get()
		if config.App.Name != "myapp" {
			t.Fatalf("expected app.name=myapp, got %s", config.App.Name)
		}
	})

	t.Run("sensitive field to sensitive layer succeeds", func(t *testing.T) {
		store := New[sensitiveTestConfig]()

		// Add a sensitive layer
		if err := store.Add(mapdata.New("secrets", map[string]any{
			"credentials": map[string]any{"api_key": "old-key"},
		}), WithSensitive()); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		// Write sensitive field to sensitive layer - should succeed
		err := store.SetTo("secrets", "/credentials/api_key", "new-key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the value was set
		config := store.Get()
		if config.Credentials.APIKey != "new-key" {
			t.Fatalf("expected api_key=new-key, got %s", config.Credentials.APIKey)
		}
	})

	t.Run("normal field to normal layer succeeds", func(t *testing.T) {
		store := New[sensitiveTestConfig]()

		// Add a normal layer
		if err := store.Add(mapdata.New("normal", map[string]any{
			"app": map[string]any{"name": "old-name"},
		})); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		// Write normal field to normal layer - should succeed
		err := store.SetTo("normal", "/app/name", "new-name")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the value was set
		config := store.Get()
		if config.App.Name != "new-name" {
			t.Fatalf("expected app.name=new-name, got %s", config.App.Name)
		}
	})
}

func TestSensitiveFieldExplicitOnly(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit sensitive field to normal layer fails", func(t *testing.T) {
		store := New[sensitiveTestConfig]()

		// Add a normal layer
		if err := store.Add(mapdata.New("normal", map[string]any{})); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		// /credentials/password has explicit sensitive tag - should fail
		err := store.SetTo("normal", "/credentials/password", "secret")
		if err == nil {
			t.Fatal("expected error when writing explicit sensitive field to normal layer")
		}
		if !errors.Is(err, ErrSensitiveFieldToNormalLayer) {
			t.Fatalf("expected ErrSensitiveFieldToNormalLayer, got: %v", err)
		}
	})

	t.Run("non-sensitive field in same struct can be written to normal layer", func(t *testing.T) {
		store := New[sensitiveTestConfig]()

		// Add a normal layer
		if err := store.Add(mapdata.New("normal", map[string]any{
			"credentials": map[string]any{},
		})); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		// /credentials/public_key has NO sensitive tag, so it should be writable to normal layer
		err := store.SetTo("normal", "/credentials/public_key", "public-key-value")
		if err != nil {
			t.Fatalf("unexpected error writing non-sensitive field to normal layer: %v", err)
		}

		config := store.Get()
		if config.Credentials.PublicKey != "public-key-value" {
			t.Fatalf("expected public_key=public-key-value, got %s", config.Credentials.PublicKey)
		}
	})

	t.Run("explicit sensitive field in any struct requires sensitive layer", func(t *testing.T) {
		store := New[sensitiveTestConfig]()

		// Add a normal layer
		if err := store.Add(mapdata.New("normal", map[string]any{
			"database": map[string]any{"host": "localhost"},
		})); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		// /database/host is not sensitive - should work
		err := store.SetTo("normal", "/database/host", "newhost")
		if err != nil {
			t.Fatalf("unexpected error writing non-sensitive field: %v", err)
		}

		// /database/password has explicit sensitive tag - should fail
		err = store.SetTo("normal", "/database/password", "secret")
		if err == nil {
			t.Fatal("expected error when writing sensitive field to normal layer")
		}
		if !errors.Is(err, ErrSensitiveFieldToNormalLayer) {
			t.Fatalf("expected ErrSensitiveFieldToNormalLayer, got: %v", err)
		}
	})
}

func TestLayerInfoSensitive(t *testing.T) {
	store := New[sensitiveTestConfig]()

	// Add normal layer
	if err := store.Add(mapdata.New("normal", map[string]any{}), WithPriority(0)); err != nil {
		t.Fatalf("Add error: %v", err)
	}

	// Add sensitive layer
	if err := store.Add(mapdata.New("secrets", map[string]any{}), WithPriority(10), WithSensitive()); err != nil {
		t.Fatalf("Add error: %v", err)
	}

	layers := store.ListLayers()
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}

	// Find and check normal layer
	var normalLayer, sensitiveLayer LayerInfo
	for _, l := range layers {
		if l.Name() == "normal" {
			normalLayer = l
		} else if l.Name() == "secrets" {
			sensitiveLayer = l
		}
	}

	if normalLayer == nil {
		t.Fatal("normal layer not found")
	}
	if sensitiveLayer == nil {
		t.Fatal("secrets layer not found")
	}

	if normalLayer.Sensitive() {
		t.Error("normal layer should not be sensitive")
	}
	if !sensitiveLayer.Sensitive() {
		t.Error("secrets layer should be sensitive")
	}
}

func TestMappingTableIsSensitive(t *testing.T) {
	// Test the MappingTable.IsSensitive method directly
	// Note: With explicit-only sensitive marking, only leaf fields with
	// explicit `jubako:"sensitive"` tag are considered sensitive.
	table := buildMappingTable(reflect.TypeOf(sensitiveTestConfig{}), DefaultTagDelimiter, DefaultFieldTagName)

	tests := []struct {
		path          string
		wantSensitive bool
	}{
		{"/app", false},
		{"/app/name", false},
		{"/credentials", false},             // Container - not sensitive (no inheritance)
		{"/credentials/api_key", true},      // Explicit sensitive tag
		{"/credentials/password", true},     // Explicit sensitive tag
		{"/credentials/public_key", false},  // No sensitive tag
		{"/database", false},
		{"/database/host", false},
		{"/database/password", true},        // Explicit sensitive tag
	}

	for _, tt := range tests {
		got := table.IsSensitive(tt.path)
		if got != tt.wantSensitive {
			t.Errorf("IsSensitive(%q) = %v, want %v", tt.path, got, tt.wantSensitive)
		}
	}
}

func TestSensitiveWithMapType(t *testing.T) {
	// Test that map[string]any works (no struct type)
	store := New[map[string]any]()

	ctx := context.Background()
	if err := store.Add(mapdata.New("test", map[string]any{"foo": "bar"})); err != nil {
		t.Fatalf("Add error: %v", err)
	}

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// Should work without errors (no sensitivity restrictions for map[string]any)
	err := store.SetTo("test", "/foo", "baz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSensitiveMasking(t *testing.T) {
	ctx := context.Background()

	t.Run("GetAt masks sensitive values", func(t *testing.T) {
		store := New[sensitiveTestConfig](WithSensitiveMaskString("[REDACTED]"))

		if err := store.Add(mapdata.New("test", map[string]any{
			"app":         map[string]any{"name": "myapp"},
			"credentials": map[string]any{"api_key": "secret-key-123"},
		})); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		// Non-sensitive field should not be masked
		rv := store.GetAt("/app/name")
		if rv.Value != "myapp" {
			t.Errorf("expected app.name=myapp, got %v", rv.Value)
		}
		if rv.Masked {
			t.Error("app.name should not be masked")
		}

		// Sensitive field should be masked
		rv = store.GetAt("/credentials/api_key")
		if rv.Value != "[REDACTED]" {
			t.Errorf("expected masked value [REDACTED], got %v", rv.Value)
		}
		if !rv.Masked {
			t.Error("credentials.api_key should be masked")
		}
	})

	t.Run("GetAtUnmasked returns original value", func(t *testing.T) {
		store := New[sensitiveTestConfig](WithSensitiveMaskString("[REDACTED]"))

		if err := store.Add(mapdata.New("test", map[string]any{
			"credentials": map[string]any{"api_key": "secret-key-123"},
		})); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		// GetAtUnmasked should return original value
		rv := store.GetAtUnmasked("/credentials/api_key")
		if rv.Value != "secret-key-123" {
			t.Errorf("expected original value secret-key-123, got %v", rv.Value)
		}
		if rv.Masked {
			t.Error("GetAtUnmasked should not set Masked=true")
		}
	})

	t.Run("Walk masks sensitive values", func(t *testing.T) {
		store := New[sensitiveTestConfig](WithSensitiveMaskString("****"))

		if err := store.Add(mapdata.New("test", map[string]any{
			"app":         map[string]any{"name": "myapp"},
			"credentials": map[string]any{"api_key": "secret"},
		})); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		maskedPaths := make(map[string]bool)
		store.Walk(func(ctx WalkContext) bool {
			rv := ctx.Value()
			if rv.Masked {
				maskedPaths[ctx.Path] = true
				if rv.Value != "****" {
					t.Errorf("expected masked value ****, got %v at %s", rv.Value, ctx.Path)
				}
			}
			return true
		})

		if !maskedPaths["/credentials/api_key"] {
			t.Error("expected /credentials/api_key to be masked in Walk")
		}
	})

	t.Run("WalkContext.IsSensitive returns correct value", func(t *testing.T) {
		store := New[sensitiveTestConfig]()

		if err := store.Add(mapdata.New("test", map[string]any{
			"app":         map[string]any{"name": "myapp"},
			"credentials": map[string]any{"api_key": "secret"},
		})); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		sensitivePaths := make(map[string]bool)
		store.Walk(func(ctx WalkContext) bool {
			if ctx.IsSensitive() {
				sensitivePaths[ctx.Path] = true
			}
			return true
		})

		if sensitivePaths["/app/name"] {
			t.Error("/app/name should not be sensitive")
		}
		if !sensitivePaths["/credentials/api_key"] {
			t.Error("/credentials/api_key should be sensitive")
		}
	})

	t.Run("custom mask function", func(t *testing.T) {
		store := New[sensitiveTestConfig](WithSensitiveMask(func(value any) any {
			if s, ok := value.(string); ok && len(s) > 4 {
				return s[:2] + "***" + s[len(s)-2:]
			}
			return "***"
		}))

		if err := store.Add(mapdata.New("test", map[string]any{
			"credentials": map[string]any{"api_key": "secret-key-123"},
		})); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		rv := store.GetAt("/credentials/api_key")
		expected := "se***23"
		if rv.Value != expected {
			t.Errorf("expected %s, got %v", expected, rv.Value)
		}
	})

	t.Run("no masking without mask option", func(t *testing.T) {
		store := New[sensitiveTestConfig]() // No mask option

		if err := store.Add(mapdata.New("test", map[string]any{
			"credentials": map[string]any{"api_key": "secret-key-123"},
		})); err != nil {
			t.Fatalf("Add error: %v", err)
		}

		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load error: %v", err)
		}

		// Without mask option, value should not be masked
		rv := store.GetAt("/credentials/api_key")
		if rv.Value != "secret-key-123" {
			t.Errorf("expected original value, got %v", rv.Value)
		}
		if rv.Masked {
			t.Error("value should not be masked without mask option")
		}
	})
}
