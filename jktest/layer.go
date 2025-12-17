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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/watcher"
)

// LayerFactory creates a Layer initialized with the given test data.
// The factory is called for each test case to ensure test isolation.
type LayerFactory func(data map[string]any) layer.Layer

// LayerTesterOption configures LayerTester behavior.
type LayerTesterOption func(*LayerTester)

// SkipNullTest skips the null value test.
// Use this for layers that don't support null values (e.g., TOML format).
// The reason parameter is required to document why the test is skipped.
func SkipNullTest(reason string) LayerTesterOption {
	return func(lt *LayerTester) {
		lt.skipNullReason = reason
	}
}

// SkipArrayTest skips the array load test.
// Use this for layers that don't support arrays (e.g., env layer).
// The reason parameter is required to document why the test is skipped.
func SkipArrayTest(reason string) LayerTesterOption {
	return func(lt *LayerTester) {
		lt.skipArrayReason = reason
	}
}

// SkipSaveArrayTest skips the array save operations test.
// Use this for layers that support reading arrays but not writing via index.
// The reason parameter is required to document why the test is skipped.
func SkipSaveArrayTest(reason string) LayerTesterOption {
	return func(lt *LayerTester) {
		lt.skipSaveArrayReason = reason
	}
}

// SkipWatchTest skips the watch tests.
// Use this for layers that don't support watching or have custom watch behavior.
// The reason parameter is required to document why the test is skipped.
func SkipWatchTest(reason string) LayerTesterOption {
	return func(lt *LayerTester) {
		lt.skipWatchReason = reason
	}
}

// LayerTester provides utilities to verify Layer implementations.
type LayerTester struct {
	t                   *testing.T
	factory             LayerFactory
	skipNullReason      string
	skipArrayReason     string
	skipSaveArrayReason string
	skipWatchReason     string
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
	lt.t.Run("SaveEmptyChangeset", lt.testSaveEmptyChangeset)
	lt.t.Run("SaveEmptyInput", lt.testSaveEmptyInput)
	lt.t.Run("SaveSkipsInvalidPaths", lt.testSaveSkipsInvalidPaths)
	lt.t.Run("SaveArrayOperations", lt.testSaveArrayOperations)
	lt.t.Run("Watch", lt.testWatch)
}

// testLoad verifies Load returns correct data.
func (lt *LayerTester) testLoad(t *testing.T) {
	testData := map[string]any{"key": "value"}
	l := lt.factory(testData)

	data, err := l.Load(context.Background())
	requireNoError(t, err, "Load error = %v", err)
	require(t, data != nil, "Load returned nil map")
	check(t, data["key"] == "value", "Load returned %v, want map with key=value", data)
}

// testLoadEmpty verifies Load handles empty/nil data correctly.
func (lt *LayerTester) testLoadEmpty(t *testing.T) {
	// Test with empty map
	t.Run("empty_map", func(t *testing.T) {
		l := lt.factory(map[string]any{})

		data, err := l.Load(context.Background())
		requireNoError(t, err, "Load error = %v", err)
		require(t, data != nil, "Load returned nil for empty map")
		check(t, len(data) == 0, "Load returned non-empty map for empty input: %v", data)
	})

	// Test with nil map (should return empty map, not nil)
	t.Run("nil_map", func(t *testing.T) {
		l := lt.factory(nil)

		data, err := l.Load(context.Background())
		requireNoError(t, err, "Load error = %v", err)
		require(t, data != nil, "Load returned nil for nil map")
		check(t, len(data) == 0, "Load returned non-empty map for nil input: %v", data)
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
	requireNoError(t, err, "Load error = %v", err)

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
			require(t, ok, "GetPath(%q) returned ok=false", tt.path)
			check(t, valuesEqual(got, tt.value), "GetPath(%q) = %v (%T), want %v (%T)",
				tt.path, got, got, tt.value, tt.value)
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
	requireNoError(t, err, "Load error = %v", err)

	// Get nested value
	got, ok := jsonptr.GetPath(data, "/a/b/c")
	require(t, ok, "GetPath(/a/b/c) returned ok=false")
	check(t, got == "deep", "GetPath(/a/b/c) = %v, want \"deep\"", got)

	// Get intermediate container
	container, ok := jsonptr.GetPath(data, "/a/b")
	require(t, ok, "GetPath(/a/b) returned ok=false")
	containerMap, isMap := container.(map[string]any)
	require(t, isMap, "GetPath(/a/b) returned %T, want map[string]any", container)
	check(t, containerMap["c"] == "deep", "container[\"c\"] = %v, want \"deep\"", containerMap["c"])

	// Get root should contain nested structure
	root, ok := jsonptr.GetPath(data, "")
	require(t, ok, "GetPath(\"\") returned ok=false")
	rootMap := root.(map[string]any)
	aMap := rootMap["a"].(map[string]any)
	bMap := aMap["b"].(map[string]any)
	check(t, bMap["c"] == "deep", "root structure incorrect: %v", rootMap)
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

	// Build test data, excluding null_value if skipNullReason is set
	// This is important for formats like TOML that don't support null values
	// and would fail during MarshalTestData if null is included.
	testData := map[string]any{
		"empty_string": "",
		"zero_int":     0,
		"false_bool":   false,
	}
	if lt.skipNullReason == "" {
		testData["null_value"] = nil
	}
	l := lt.factory(testData)

	data, err := l.Load(context.Background())
	requireNoError(t, err, "Load error = %v", err)

	// Verify all values
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipNull && lt.skipNullReason != "" {
				t.Skip(lt.skipNullReason)
			}

			path := "/" + tt.key
			got, ok := jsonptr.GetPath(data, path)
			require(t, ok, "GetPath(%q) returned ok=false", path)
			check(t, valuesEqual(got, tt.value), "GetPath(%q) = %v (%T), want %v (%T)",
				path, got, got, tt.value, tt.value)
		})
	}

	// Verify non-existent path returns ok=false
	t.Run("missing", func(t *testing.T) {
		_, ok := jsonptr.GetPath(data, "/nonexistent")
		check(t, !ok, "GetPath(/nonexistent) returned ok=true, want false")
	})

	// Verify root contains all values
	t.Run("root_contains_all", func(t *testing.T) {
		root, ok := jsonptr.GetPath(data, "")
		require(t, ok, "GetPath(\"\") returned ok=false")

		rootMap, isMap := root.(map[string]any)
		require(t, isMap, "GetPath(\"\") returned %T, want map[string]any", root)

		for _, tt := range tests {
			// Skip null test if configured
			if tt.skipNull && lt.skipNullReason != "" {
				continue
			}

			val, exists := rootMap[tt.key]
			require(t, exists, "root missing key %q", tt.key)
			check(t, valuesEqual(val, tt.value), "root[%q] = %v (%T), want %v (%T)",
				tt.key, val, val, tt.value, tt.value)
		}
	})
}

// testArrayPaths verifies array/slice path operations work correctly.
func (lt *LayerTester) testArrayPaths(t *testing.T) {
	if lt.skipArrayReason != "" {
		t.Skip(lt.skipArrayReason)
	}

	testData := map[string]any{"items": []any{"a", "b", "c"}}
	l := lt.factory(testData)

	data, err := l.Load(context.Background())
	requireNoError(t, err, "Load error = %v", err)

	// Get entire array
	got, ok := jsonptr.GetPath(data, "/items")
	require(t, ok, "GetPath(/items) returned ok=false")

	gotArr, isArr := got.([]any)
	require(t, isArr, "GetPath(/items) returned %T, want []any", got)

	require(t, len(gotArr) == 3, "len(items) = %d, want 3", len(gotArr))

	// Get array element by index
	elem, ok := jsonptr.GetPath(data, "/items/0")
	require(t, ok, "GetPath(/items/0) returned ok=false")
	check(t, elem == "a", "GetPath(/items/0) = %v, want \"a\"", elem)

	// Get root should contain array
	root, ok := jsonptr.GetPath(data, "")
	require(t, ok, "GetPath(\"\") returned ok=false")
	rootMap := root.(map[string]any)
	rootItems, ok := rootMap["items"].([]any)
	require(t, ok, "root[\"items\"] is %T, want []any", rootMap["items"])
	check(t, len(rootItems) == 3, "len(root[\"items\"]) = %d, want 3", len(rootItems))
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

		err := l.Save(ctx, patches)
		requireNoError(t, err, "Save error = %v", err)

		data, err := l.Load(ctx)
		requireNoError(t, err, "Load after Save error = %v", err)
		check(t, data["new_key"] == "new_value", "after Add, new_key = %v, want new_value", data["new_key"])
		check(t, data["existing"] == "value", "after Add, existing = %v, want value", data["existing"])
	})

	// Apply replace operation
	t.Run("Replace", func(t *testing.T) {
		l := lt.factory(initialData)
		var patches document.JSONPatchSet
		patches.Replace("/existing", "replaced")

		err := l.Save(ctx, patches)
		requireNoError(t, err, "Save error = %v", err)

		data, err := l.Load(ctx)
		requireNoError(t, err, "Load after Save error = %v", err)
		check(t, data["existing"] == "replaced", "after Replace, existing = %v, want replaced", data["existing"])
	})

	// Apply remove operation
	t.Run("Remove", func(t *testing.T) {
		l := lt.factory(initialData)
		var patches document.JSONPatchSet
		patches.Remove("/existing")

		err := l.Save(ctx, patches)
		requireNoError(t, err, "Save error = %v", err)

		data, err := l.Load(ctx)
		requireNoError(t, err, "Load after Save error = %v", err)
		_, exists := data["existing"]
		check(t, !exists, "after Remove, existing key still present")
	})

	// Apply nested add operation
	t.Run("NestedAdd", func(t *testing.T) {
		l := lt.factory(initialData)
		var patches document.JSONPatchSet
		patches.Add("/x/y/z", "nested_value")

		err := l.Save(ctx, patches)
		requireNoError(t, err, "Save error = %v", err)

		data, err := l.Load(ctx)
		requireNoError(t, err, "Load after Save error = %v", err)

		got, ok := jsonptr.GetPath(data, "/x/y/z")
		require(t, ok, "GetPath(/x/y/z) returned ok=false after Save")
		check(t, got == "nested_value", "GetPath(/x/y/z) = %v, want \"nested_value\"", got)
	})
}

// testSaveEmptyChangeset verifies Save with empty/nil changeset preserves data.
func (lt *LayerTester) testSaveEmptyChangeset(t *testing.T) {
	initialData := map[string]any{"existing": "value"}
	l := lt.factory(initialData)

	if !l.CanSave() {
		t.Skip("layer does not support Save")
	}

	ctx := context.Background()

	// Save with nil changeset
	t.Run("nil_changeset", func(t *testing.T) {
		l := lt.factory(initialData)

		err := l.Save(ctx, nil)
		requireNoError(t, err, "Save(nil) error = %v", err)

		data, err := l.Load(ctx)
		requireNoError(t, err, "Load after Save error = %v", err)
		check(t, data["existing"] == "value", "after Save(nil), existing = %v, want value", data["existing"])
	})

	// Save with empty changeset
	t.Run("empty_changeset", func(t *testing.T) {
		l := lt.factory(initialData)

		err := l.Save(ctx, document.JSONPatchSet{})
		requireNoError(t, err, "Save(empty) error = %v", err)

		data, err := l.Load(ctx)
		requireNoError(t, err, "Load after Save error = %v", err)
		check(t, data["existing"] == "value", "after Save(empty), existing = %v, want value", data["existing"])
	})
}

// testSaveEmptyInput verifies Save creates new document from empty input.
func (lt *LayerTester) testSaveEmptyInput(t *testing.T) {
	// Start with empty data
	l := lt.factory(nil)

	if !l.CanSave() {
		t.Skip("layer does not support Save")
	}

	ctx := context.Background()

	var patches document.JSONPatchSet
	patches.Add("/new_key", "new_value")

	err := l.Save(ctx, patches)
	requireNoError(t, err, "Save error = %v", err)

	data, err := l.Load(ctx)
	requireNoError(t, err, "Load after Save error = %v", err)
	check(t, data["new_key"] == "new_value", "after Save, new_key = %v, want new_value", data["new_key"])
}

// testSaveSkipsInvalidPaths verifies Save skips invalid paths without error.
func (lt *LayerTester) testSaveSkipsInvalidPaths(t *testing.T) {
	initialData := map[string]any{"existing": "value"}
	l := lt.factory(initialData)

	if !l.CanSave() {
		t.Skip("layer does not support Save")
	}

	ctx := context.Background()

	// Mix of valid and invalid operations
	var patches document.JSONPatchSet
	patches = append(patches, document.JSONPatch{Op: document.PatchOpAdd, Path: "relative", Value: 1})    // invalid: relative path
	patches = append(patches, document.JSONPatch{Op: document.PatchOpAdd, Path: "", Value: 1})            // invalid: root path
	patches = append(patches, document.JSONPatch{Op: document.PatchOpRemove, Path: "also_relative"})      // invalid: relative path
	patches.Add("/valid_key", "valid_value")                                                               // valid
	patches.Replace("/existing", "replaced")                                                               // valid

	err := l.Save(ctx, patches)
	requireNoError(t, err, "Save error = %v", err)

	data, err := l.Load(ctx)
	requireNoError(t, err, "Load after Save error = %v", err)

	// Valid operations should be applied
	check(t, data["valid_key"] == "valid_value", "valid_key = %v, want valid_value", data["valid_key"])
	check(t, data["existing"] == "replaced", "existing = %v, want replaced", data["existing"])

	// Invalid operations should be skipped (keys should not exist)
	_, hasRelative := data["relative"]
	check(t, !hasRelative, "relative key should not exist")
	_, hasAlsoRelative := data["also_relative"]
	check(t, !hasAlsoRelative, "also_relative key should not exist")
}

// testSaveArrayOperations verifies Save handles array Add/Replace/Remove operations.
func (lt *LayerTester) testSaveArrayOperations(t *testing.T) {
	if lt.skipArrayReason != "" {
		t.Skip(lt.skipArrayReason)
	}
	if lt.skipSaveArrayReason != "" {
		t.Skip(lt.skipSaveArrayReason)
	}

	ctx := context.Background()

	// Test adding to array
	t.Run("AddToArray", func(t *testing.T) {
		initialData := map[string]any{"items": []any{"a", "b"}}
		l := lt.factory(initialData)

		if !l.CanSave() {
			t.Skip("layer does not support Save")
		}

		var patches document.JSONPatchSet
		patches.Add("/items/2", "c")

		err := l.Save(ctx, patches)
		requireNoError(t, err, "Save error = %v", err)

		data, err := l.Load(ctx)
		requireNoError(t, err, "Load after Save error = %v", err)

		items, ok := data["items"].([]any)
		require(t, ok, "items is %T, want []any", data["items"])
		require(t, len(items) == 3, "len(items) = %d, want 3", len(items))
		check(t, items[2] == "c", "items[2] = %v, want c", items[2])
	})

	// Test replacing array element
	t.Run("ReplaceArrayElement", func(t *testing.T) {
		initialData := map[string]any{"items": []any{"a", "b", "c"}}
		l := lt.factory(initialData)

		if !l.CanSave() {
			t.Skip("layer does not support Save")
		}

		var patches document.JSONPatchSet
		patches.Replace("/items/1", "B")

		err := l.Save(ctx, patches)
		requireNoError(t, err, "Save error = %v", err)

		data, err := l.Load(ctx)
		requireNoError(t, err, "Load after Save error = %v", err)

		items, ok := data["items"].([]any)
		require(t, ok, "items is %T, want []any", data["items"])
		check(t, items[1] == "B", "items[1] = %v, want B", items[1])
	})

	// Test removing array element
	t.Run("RemoveArrayElement", func(t *testing.T) {
		initialData := map[string]any{"items": []any{"a", "b", "c"}}
		l := lt.factory(initialData)

		if !l.CanSave() {
			t.Skip("layer does not support Save")
		}

		var patches document.JSONPatchSet
		patches.Remove("/items/1")

		err := l.Save(ctx, patches)
		requireNoError(t, err, "Save error = %v", err)

		data, err := l.Load(ctx)
		requireNoError(t, err, "Load after Save error = %v", err)

		items, ok := data["items"].([]any)
		require(t, ok, "items is %T, want []any", data["items"])
		require(t, len(items) == 2, "len(items) = %d, want 2", len(items))
		check(t, items[0] == "a", "items[0] = %v, want a", items[0])
		check(t, items[1] == "c", "items[1] = %v, want c", items[1])
	})

	// Test nested array operations
	t.Run("NestedArrayAdd", func(t *testing.T) {
		initialData := map[string]any{"matrix": []any{[]any{1, 2}}}
		l := lt.factory(initialData)

		if !l.CanSave() {
			t.Skip("layer does not support Save")
		}

		var patches document.JSONPatchSet
		patches.Add("/matrix/0/2", 3)

		err := l.Save(ctx, patches)
		requireNoError(t, err, "Save error = %v", err)

		data, err := l.Load(ctx)
		requireNoError(t, err, "Load after Save error = %v", err)

		matrix, ok := data["matrix"].([]any)
		require(t, ok, "matrix is %T, want []any", data["matrix"])
		row, ok := matrix[0].([]any)
		require(t, ok, "matrix[0] is %T, want []any", matrix[0])
		require(t, len(row) == 3, "len(matrix[0]) = %d, want 3", len(row))
		check(t, valuesEqual(row[2], 3), "matrix[0][2] = %v, want 3", row[2])
	})
}

// testWatch verifies Watch behavior for Layer implementations.
func (lt *LayerTester) testWatch(t *testing.T) {
	if lt.skipWatchReason != "" {
		t.Skip(lt.skipWatchReason)
	}

	testData := map[string]any{"key": "value"}
	l := lt.factory(testData)

	t.Run("WatchReturnsValidWatcher", func(t *testing.T) {
		w, err := l.Watch()
		requireNoError(t, err, "Watch() error = %v", err)
		require(t, w != nil, "Watch() returned nil watcher")
	})

	t.Run("WatchResultsNilBeforeStart", func(t *testing.T) {
		l := lt.factory(testData)
		w, err := l.Watch()
		requireNoError(t, err, "Watch() error = %v", err)

		// Results() should return nil before Start() is called
		results := w.Results()
		check(t, results == nil, "Results() should return nil before Start() is called, got %v", results)
	})

	t.Run("WatchStartStop", func(t *testing.T) {
		l := lt.factory(testData)

		cfg := watcher.NewWatchConfig(
			watcher.WithPollInterval(100 * time.Millisecond),
		)
		w, err := l.Watch(layer.WithBaseConfig(cfg))
		requireNoError(t, err, "Watch() error = %v", err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = w.Start(ctx)
		requireNoError(t, err, "Start() error = %v", err)

		// Results() should return a non-nil channel after Start()
		results := w.Results()
		require(t, results != nil, "Results() returned nil after Start()")

		// Stop should work without error
		err = w.Stop(context.Background())
		requireNoError(t, err, "Stop() error = %v", err)
	})

	t.Run("WatchCanBeCalledMultipleTimes", func(t *testing.T) {
		l := lt.factory(testData)

		// Calling Watch() multiple times should not cause issues
		w1, err := l.Watch()
		requireNoError(t, err, "First Watch() error = %v", err)
		require(t, w1 != nil, "First Watch() returned nil watcher")

		w2, err := l.Watch()
		requireNoError(t, err, "Second Watch() error = %v", err)
		require(t, w2 != nil, "Second Watch() returned nil watcher")
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
