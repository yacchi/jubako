// Package jktest provides testing utilities for jubako implementations.
//
// Example usage with mapdata layer:
//
//	import "github.com/yacchi/jubako/jktest"
//	import "github.com/yacchi/jubako/layer/mapdata"
//
//	func TestMapDataLayer_Compliance(t *testing.T) {
//	    factory := func(data map[string]any) layer.Layer {
//	        return mapdata.New("test", data)
//	    }
//	    jktest.NewLayerTester(t, factory).TestAll()
//	}
//
// Example usage with env layer:
//
//	func TestEnvLayer_Compliance(t *testing.T) {
//	    factory := func(data map[string]any) layer.Layer {
//	        // Convert map data to environment variables
//	        envVars := jktest.MapToEnvVars("TEST_", "_", data)
//	        return env.New("test", "TEST_", env.WithEnvironFunc(func() []string {
//	            return envVars
//	        }))
//	    }
//	    jktest.NewLayerTester(t, factory).TestAll()
//	}
package jktest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source"
)

// LayerFactory creates a Layer initialized with the given test data.
// The factory is called for each test case to ensure test isolation.
type LayerFactory func(data map[string]any) layer.Layer

// LayerTesterOption configures LayerTester behavior.
type LayerTesterOption func(*LayerTester)

// SkipNullTest skips the null value test.
// Use this for layers that don't support null values (e.g., env layer).
func SkipNullTest() LayerTesterOption {
	return func(lt *LayerTester) {
		lt.skipNull = true
	}
}

// SkipArrayTest skips the array test.
// Use this for layers that don't support arrays (e.g., env layer).
func SkipArrayTest() LayerTesterOption {
	return func(lt *LayerTester) {
		lt.skipArray = true
	}
}

// LayerTester provides utilities to verify Layer implementations.
type LayerTester struct {
	t         *testing.T
	factory   LayerFactory
	skipNull  bool
	skipArray bool
}

// NewLayerTester creates a LayerTester for the given LayerFactory.
// The factory will be used to create new Layer instances for each test.
func NewLayerTester(t *testing.T, factory LayerFactory, opts ...LayerTesterOption) *LayerTester {
	lt := &LayerTester{
		t:       t,
		factory: factory,
	}
	for _, opt := range opts {
		opt(lt)
	}
	return lt
}

// TestAll runs all standard compliance tests.
func (lt *LayerTester) TestAll() {
	lt.t.Run("Load", lt.testLoad)
	lt.t.Run("LoadEmpty", lt.testLoadEmpty)
	lt.t.Run("LoadPath", lt.testLoadPath)
	lt.t.Run("NestedPaths", lt.testNestedPaths)
	lt.t.Run("SpecialValues", lt.testSpecialValues)
	lt.t.Run("ArrayPaths", lt.testArrayPaths)
	lt.t.Run("Save", lt.testSave)
}

// testLoad verifies Load returns correct data.
func (lt *LayerTester) testLoad(t *testing.T) {
	testData := map[string]any{"key": "value"}
	l := lt.factory(testData)

	data, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}

	if data == nil {
		t.Fatal("Load returned nil map")
	}

	if data["key"] != "value" {
		t.Errorf("Load returned %v, want map with key=value", data)
	}
}

// testLoadEmpty verifies Load handles empty/nil data correctly.
func (lt *LayerTester) testLoadEmpty(t *testing.T) {
	// Test with empty map
	t.Run("empty_map", func(t *testing.T) {
		l := lt.factory(map[string]any{})

		data, err := l.Load(context.Background())
		if err != nil {
			t.Fatalf("Load error = %v", err)
		}

		if data == nil {
			t.Fatal("Load returned nil for empty map")
		}
		if len(data) != 0 {
			t.Errorf("Load returned non-empty map for empty input: %v", data)
		}
	})

	// Test with nil map (should return empty map, not nil)
	t.Run("nil_map", func(t *testing.T) {
		l := lt.factory(nil)

		data, err := l.Load(context.Background())
		if err != nil {
			t.Fatalf("Load error = %v", err)
		}

		if data == nil {
			t.Fatal("Load returned nil for nil map")
		}
		if len(data) != 0 {
			t.Errorf("Load returned non-empty map for nil input: %v", data)
		}
	})
}

// testLoadPath verifies values can be read from loaded data using jsonptr.
func (lt *LayerTester) testLoadPath(t *testing.T) {
	testData := map[string]any{
		"string": "hello",
		"int":    42,
		"float":  3.14,
		"bool":   true,
	}
	l := lt.factory(testData)

	data, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}

	tests := []struct {
		path  string
		value any
	}{
		{"/string", "hello"},
		{"/int", 42},
		{"/float", 3.14},
		{"/bool", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, ok := jsonptr.GetPath(data, tt.path)
			if !ok {
				t.Fatalf("GetPath(%q) returned ok=false", tt.path)
			}

			if !valuesEqual(got, tt.value) {
				t.Errorf("GetPath(%q) = %v (%T), want %v (%T)",
					tt.path, got, got, tt.value, tt.value)
			}
		})
	}
}

// testNestedPaths verifies nested path operations work correctly.
func (lt *LayerTester) testNestedPaths(t *testing.T) {
	testData := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "deep",
			},
		},
	}
	l := lt.factory(testData)

	data, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}

	// Get nested value
	got, ok := jsonptr.GetPath(data, "/a/b/c")
	if !ok {
		t.Fatal("GetPath(/a/b/c) returned ok=false")
	}
	if got != "deep" {
		t.Errorf("GetPath(/a/b/c) = %v, want \"deep\"", got)
	}

	// Get intermediate container
	container, ok := jsonptr.GetPath(data, "/a/b")
	if !ok {
		t.Fatal("GetPath(/a/b) returned ok=false")
	}
	containerMap, isMap := container.(map[string]any)
	if !isMap {
		t.Fatalf("GetPath(/a/b) returned %T, want map[string]any", container)
	}
	if containerMap["c"] != "deep" {
		t.Errorf("container[\"c\"] = %v, want \"deep\"", containerMap["c"])
	}

	// Get root should contain nested structure
	root, ok := jsonptr.GetPath(data, "")
	if !ok {
		t.Fatal("GetPath(\"\") returned ok=false")
	}
	rootMap := root.(map[string]any)
	aMap := rootMap["a"].(map[string]any)
	bMap := aMap["b"].(map[string]any)
	if bMap["c"] != "deep" {
		t.Errorf("root structure incorrect: %v", rootMap)
	}
}

// testSpecialValues verifies null, empty string, zero, and false are handled correctly.
func (lt *LayerTester) testSpecialValues(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    any
		skipNull bool
	}{
		{"null", "null_value", nil, true},
		{"empty_string", "empty_string", "", false},
		{"zero_int", "zero_int", 0, false},
		{"false_bool", "false_bool", false, false},
	}

	// Build test data, excluding null_value if skipNull is set
	// This is important for formats like TOML that don't support null values
	// and would fail during MarshalTestData if null is included.
	testData := map[string]any{
		"empty_string": "",
		"zero_int":     0,
		"false_bool":   false,
	}
	if !lt.skipNull {
		testData["null_value"] = nil
	}
	l := lt.factory(testData)

	data, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}

	// Verify all values
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipNull && lt.skipNull {
				t.Skip("null values not supported by this layer")
			}

			path := "/" + tt.key
			got, ok := jsonptr.GetPath(data, path)
			if !ok {
				t.Fatalf("GetPath(%q) returned ok=false", path)
			}

			if !valuesEqual(got, tt.value) {
				t.Errorf("GetPath(%q) = %v (%T), want %v (%T)",
					path, got, got, tt.value, tt.value)
			}
		})
	}

	// Verify non-existent path returns ok=false
	t.Run("missing", func(t *testing.T) {
		_, ok := jsonptr.GetPath(data, "/nonexistent")
		if ok {
			t.Error("GetPath(/nonexistent) returned ok=true, want false")
		}
	})

	// Verify root contains all values
	t.Run("root_contains_all", func(t *testing.T) {
		root, ok := jsonptr.GetPath(data, "")
		if !ok {
			t.Fatal("GetPath(\"\") returned ok=false")
		}

		rootMap, isMap := root.(map[string]any)
		if !isMap {
			t.Fatalf("GetPath(\"\") returned %T, want map[string]any", root)
		}

		for _, tt := range tests {
			// Skip null test if configured
			if tt.skipNull && lt.skipNull {
				continue
			}

			val, exists := rootMap[tt.key]
			if !exists {
				t.Errorf("root missing key %q", tt.key)
				continue
			}

			if !valuesEqual(val, tt.value) {
				t.Errorf("root[%q] = %v (%T), want %v (%T)",
					tt.key, val, val, tt.value, tt.value)
			}
		}
	})
}

// testArrayPaths verifies array/slice path operations work correctly.
func (lt *LayerTester) testArrayPaths(t *testing.T) {
	if lt.skipArray {
		t.Skip("arrays not supported by this layer")
	}

	testData := map[string]any{"items": []any{"a", "b", "c"}}
	l := lt.factory(testData)

	data, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}

	// Get entire array
	got, ok := jsonptr.GetPath(data, "/items")
	if !ok {
		t.Fatal("GetPath(/items) returned ok=false")
	}

	gotArr, isArr := got.([]any)
	if !isArr {
		t.Fatalf("GetPath(/items) returned %T, want []any", got)
	}

	if len(gotArr) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(gotArr))
	}

	// Get array element by index
	elem, ok := jsonptr.GetPath(data, "/items/0")
	if !ok {
		t.Fatal("GetPath(/items/0) returned ok=false")
	}
	if elem != "a" {
		t.Errorf("GetPath(/items/0) = %v, want \"a\"", elem)
	}

	// Get root should contain array
	root, ok := jsonptr.GetPath(data, "")
	if !ok {
		t.Fatal("GetPath(\"\") returned ok=false")
	}
	rootMap := root.(map[string]any)
	rootItems, ok := rootMap["items"].([]any)
	if !ok {
		t.Fatalf("root[\"items\"] is %T, want []any", rootMap["items"])
	}
	if len(rootItems) != 3 {
		t.Errorf("len(root[\"items\"]) = %d, want 3", len(rootItems))
	}
}

// testSave verifies Save modifies data correctly (for writable layers).
func (lt *LayerTester) testSave(t *testing.T) {
	initialData := map[string]any{"existing": "value"}
	l := lt.factory(initialData)

	if !l.CanSave() {
		t.Skip("layer does not support Save")
	}

	ctx := context.Background()

	// Apply add operation
	t.Run("Add", func(t *testing.T) {
		l := lt.factory(initialData)
		var patches document.JSONPatchSet
		patches.Add("/new_key", "new_value")

		if err := l.Save(ctx, patches); err != nil {
			t.Fatalf("Save error = %v", err)
		}

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load after Save error = %v", err)
		}

		if data["new_key"] != "new_value" {
			t.Errorf("after Add, new_key = %v, want new_value", data["new_key"])
		}
		if data["existing"] != "value" {
			t.Errorf("after Add, existing = %v, want value", data["existing"])
		}
	})

	// Apply replace operation
	t.Run("Replace", func(t *testing.T) {
		l := lt.factory(initialData)
		var patches document.JSONPatchSet
		patches.Replace("/existing", "replaced")

		if err := l.Save(ctx, patches); err != nil {
			t.Fatalf("Save error = %v", err)
		}

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load after Save error = %v", err)
		}

		if data["existing"] != "replaced" {
			t.Errorf("after Replace, existing = %v, want replaced", data["existing"])
		}
	})

	// Apply remove operation
	t.Run("Remove", func(t *testing.T) {
		l := lt.factory(initialData)
		var patches document.JSONPatchSet
		patches.Remove("/existing")

		if err := l.Save(ctx, patches); err != nil {
			t.Fatalf("Save error = %v", err)
		}

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load after Save error = %v", err)
		}

		if _, exists := data["existing"]; exists {
			t.Error("after Remove, existing key still present")
		}
	})

	// Apply nested add operation
	t.Run("NestedAdd", func(t *testing.T) {
		l := lt.factory(initialData)
		var patches document.JSONPatchSet
		patches.Add("/x/y/z", "nested_value")

		if err := l.Save(ctx, patches); err != nil {
			t.Fatalf("Save error = %v", err)
		}

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load after Save error = %v", err)
		}

		got, ok := jsonptr.GetPath(data, "/x/y/z")
		if !ok {
			t.Fatal("GetPath(/x/y/z) returned ok=false after Save")
		}
		if got != "nested_value" {
			t.Errorf("GetPath(/x/y/z) = %v, want \"nested_value\"", got)
		}
	})
}

// MapToEnvVars converts a map[string]any to environment variable strings.
// This is useful for testing env layers.
//
// Example:
//
//	data := map[string]any{
//	    "server": map[string]any{
//	        "host": "localhost",
//	        "port": 8080,
//	    },
//	}
//	envVars := MapToEnvVars("APP_", "_", data)
//	// Returns: []string{"APP_SERVER_HOST=localhost", "APP_SERVER_PORT=8080"}
func MapToEnvVars(prefix, delimiter string, data map[string]any) []string {
	var result []string
	flattenMap(prefix, delimiter, "", data, &result)
	return result
}

// flattenMap recursively flattens a nested map into environment variable format.
func flattenMap(prefix, delimiter, currentPath string, data map[string]any, result *[]string) {
	for key, value := range data {
		var envKey string
		if currentPath == "" {
			envKey = strings.ToUpper(key)
		} else {
			envKey = currentPath + delimiter + strings.ToUpper(key)
		}

		switch v := value.(type) {
		case map[string]any:
			flattenMap(prefix, delimiter, envKey, v, result)
		case nil:
			// Skip nil values (env vars don't support null)
		default:
			*result = append(*result, fmt.Sprintf("%s%s=%v", prefix, envKey, v))
		}
	}
}

// isUnsupportedError checks if the error is an UnsupportedStructureError.
func isUnsupportedError(err error) bool {
	var unsupported *document.UnsupportedStructureError
	return errors.As(err, &unsupported)
}

// valuesEqual compares two values for equality, handling numeric type conversions.
func valuesEqual(got, want any) bool {
	// Handle nil
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}

	// Handle numeric type conversions (JSON/YAML may decode as different types)
	gotNum, gotIsNum := toFloat64(got)
	wantNum, wantIsNum := toFloat64(want)
	if gotIsNum && wantIsNum {
		return gotNum == wantNum
	}

	// Handle string comparison for env layer (which stores everything as strings)
	if gotStr, ok := got.(string); ok {
		if wantStr, ok := want.(string); ok {
			return gotStr == wantStr
		}
		// Compare string representation
		return gotStr == fmt.Sprintf("%v", want)
	}

	// Use reflect.DeepEqual for other types
	return reflect.DeepEqual(got, want)
}

// toFloat64 converts numeric types to float64 for comparison.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

// TestSource is an in-memory source that supports both Load and Save.
// It is useful for testing Document implementations with LayerTester.
type TestSource struct {
	data []byte
}

// Ensure TestSource implements source.Source.
var _ source.Source = (*TestSource)(nil)

// NewTestSource creates a new TestSource with the given initial data.
func NewTestSource(data []byte) *TestSource {
	return &TestSource{data: data}
}

// Load returns the current data.
func (s *TestSource) Load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]byte, len(s.data))
	copy(result, s.data)
	return result, nil
}

// Save applies the update function and stores the result.
func (s *TestSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	newData, err := updateFunc(s.data)
	if err != nil {
		return err
	}
	s.data = newData
	return nil
}

// CanSave returns true.
func (s *TestSource) CanSave() bool {
	return true
}

// DocumentLayerFactory creates a LayerFactory for testing Document implementations.
// The returned factory creates a FileLayer using TestSource and the given Document.
//
// Example:
//
//	func TestYAMLDocument_Compliance(t *testing.T) {
//	    factory := jktest.DocumentLayerFactory(yaml.New())
//	    jktest.NewLayerTester(t, factory).TestAll()
//	}
func DocumentLayerFactory(doc document.Document) LayerFactory {
	return func(data map[string]any) layer.Layer {
		// Marshal test data to bytes using the document
		bytes, err := doc.MarshalTestData(data)
		if err != nil {
			// Return a layer that will fail on Load
			return layer.New("test", NewTestSource(nil), doc)
		}
		return layer.New("test", NewTestSource(bytes), doc)
	}
}
