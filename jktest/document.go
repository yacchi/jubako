// Package jktest provides testing utilities for jubako implementations.
//
// Example usage:
//
//	import "github.com/yacchi/jubako/jktest"
//
//	func TestMyDocument_Compliance(t *testing.T) {
//	    jktest.NewDocumentTester(t, yaml.NewParser()).TestAll()
//	}
package jktest

import (
	"errors"
	"reflect"
	"testing"

	"github.com/yacchi/jubako/document"
)

// DocumentTester provides utilities to verify Document implementations.
type DocumentTester struct {
	t      *testing.T
	parser document.Parser
}

// NewDocumentTester creates a DocumentTester for the given Parser.
// The parser will be used to create new Document instances for each test.
func NewDocumentTester(t *testing.T, parser document.Parser) *DocumentTester {
	return &DocumentTester{
		t:      t,
		parser: parser,
	}
}

// newDocument creates a new empty Document using the parser.
func (dt *DocumentTester) newDocument() document.Document {
	doc, err := dt.parser.Parse(nil)
	if err != nil {
		dt.t.Fatalf("parser.Parse(nil) error = %v", err)
	}
	return doc
}

// TestAll runs all standard compliance tests.
func (dt *DocumentTester) TestAll() {
	dt.t.Run("GetRoot", dt.testGetRoot)
	dt.t.Run("SetAndGet", dt.testSetAndGet)
	dt.t.Run("Delete", dt.testDelete)
	dt.t.Run("NestedPaths", dt.testNestedPaths)
	dt.t.Run("SpecialValues", dt.testSpecialValues)
	dt.t.Run("ArrayPaths", dt.testArrayPaths)
}

// testGetRoot verifies Get("") returns the root as map[string]any.
func (dt *DocumentTester) testGetRoot(t *testing.T) {
	doc := dt.newDocument()

	// Check if format supports Set via MarshalTestData
	testData := map[string]any{"key": "value"}
	_, err := dt.parser.MarshalTestData(testData)
	if isUnsupportedError(err) {
		t.Skip("format does not support Set: " + err.Error())
	}

	// Empty document should return empty map or be ok=false
	root, ok := doc.Get("")
	if ok {
		if _, isMap := root.(map[string]any); !isMap {
			t.Errorf("Get(\"\") returned %T, want map[string]any", root)
		}
	}

	// After setting a value, Get("") should return map containing that value
	if err := doc.Set("/key", "value"); err != nil {
		if isUnsupportedError(err) {
			t.Skip("format does not support Set: " + err.Error())
		}
		t.Fatalf("Set(/key) error = %v", err)
	}

	root, ok = doc.Get("")
	if !ok {
		t.Fatal("Get(\"\") returned ok=false after Set")
	}

	rootMap, isMap := root.(map[string]any)
	if !isMap {
		t.Fatalf("Get(\"\") returned %T, want map[string]any", root)
	}

	if rootMap["key"] != "value" {
		t.Errorf("root[\"key\"] = %v, want \"value\"", rootMap["key"])
	}
}

// testSetAndGet verifies Set then Get round-trip works correctly.
func (dt *DocumentTester) testSetAndGet(t *testing.T) {
	doc := dt.newDocument()

	// Check if format supports Set
	if err := doc.Set("/test", "value"); isUnsupportedError(err) {
		t.Skip("format does not support Set: " + err.Error())
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
			if err := doc.Set(tt.path, tt.value); err != nil {
				t.Fatalf("Set(%q, %v) error = %v", tt.path, tt.value, err)
			}

			got, ok := doc.Get(tt.path)
			if !ok {
				t.Fatalf("Get(%q) returned ok=false", tt.path)
			}

			if !valuesEqual(got, tt.value) {
				t.Errorf("Get(%q) = %v (%T), want %v (%T)",
					tt.path, got, got, tt.value, tt.value)
			}
		})
	}
}

// testDelete verifies Delete removes values correctly.
func (dt *DocumentTester) testDelete(t *testing.T) {
	doc := dt.newDocument()

	// Check if format supports Set/Delete
	if err := doc.Set("/to_delete", "value"); isUnsupportedError(err) {
		t.Skip("format does not support Set: " + err.Error())
	}

	// Verify it exists
	if _, ok := doc.Get("/to_delete"); !ok {
		t.Fatal("Get returned ok=false before Delete")
	}

	// Delete
	if err := doc.Delete("/to_delete"); err != nil {
		if isUnsupportedError(err) {
			t.Skip("format does not support Delete: " + err.Error())
		}
		t.Fatalf("Delete error = %v", err)
	}

	// Verify it's gone
	if _, ok := doc.Get("/to_delete"); ok {
		t.Error("Get returned ok=true after Delete")
	}

	// Delete non-existent path should not error (or return unsupported)
	if err := doc.Delete("/nonexistent"); err != nil && !isUnsupportedError(err) {
		t.Errorf("Delete(/nonexistent) error = %v, want nil", err)
	}
}

// testNestedPaths verifies nested path operations work correctly.
func (dt *DocumentTester) testNestedPaths(t *testing.T) {
	doc := dt.newDocument()

	// Set nested value
	if err := doc.Set("/a/b/c", "deep"); err != nil {
		if isUnsupportedError(err) {
			t.Skip("format does not support Set: " + err.Error())
		}
		t.Fatalf("Set(/a/b/c) error = %v", err)
	}

	// Get nested value
	got, ok := doc.Get("/a/b/c")
	if !ok {
		t.Fatal("Get(/a/b/c) returned ok=false")
	}
	if got != "deep" {
		t.Errorf("Get(/a/b/c) = %v, want \"deep\"", got)
	}

	// Get intermediate container
	container, ok := doc.Get("/a/b")
	if !ok {
		t.Fatal("Get(/a/b) returned ok=false")
	}
	containerMap, isMap := container.(map[string]any)
	if !isMap {
		t.Fatalf("Get(/a/b) returned %T, want map[string]any", container)
	}
	if containerMap["c"] != "deep" {
		t.Errorf("container[\"c\"] = %v, want \"deep\"", containerMap["c"])
	}

	// Get root should contain nested structure
	root, ok := doc.Get("")
	if !ok {
		t.Fatal("Get(\"\") returned ok=false")
	}
	rootMap := root.(map[string]any)
	aMap := rootMap["a"].(map[string]any)
	bMap := aMap["b"].(map[string]any)
	if bMap["c"] != "deep" {
		t.Errorf("root structure incorrect: %v", rootMap)
	}
}

// testSpecialValues verifies null, empty string, zero, and false are handled correctly.
func (dt *DocumentTester) testSpecialValues(t *testing.T) {
	doc := dt.newDocument()

	tests := []struct {
		name  string
		path  string
		value any
	}{
		{"null", "/null_value", nil},
		{"empty_string", "/empty_string", ""},
		{"zero_int", "/zero_int", 0},
		{"false_bool", "/false_bool", false},
	}

	// Check if format supports special values via MarshalTestData
	testData := map[string]any{
		"null_value":   nil,
		"empty_string": "",
		"zero_int":     0,
		"false_bool":   false,
	}
	_, err := dt.parser.MarshalTestData(testData)
	if isUnsupportedError(err) {
		t.Skip("format does not support special values: " + err.Error())
	}

	// Set all values
	for _, tt := range tests {
		if err := doc.Set(tt.path, tt.value); err != nil {
			t.Fatalf("Set(%q, %v) error = %v", tt.path, tt.value, err)
		}
	}

	// Verify all values
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := doc.Get(tt.path)
			if !ok {
				t.Fatalf("Get(%q) returned ok=false", tt.path)
			}

			if !valuesEqual(got, tt.value) {
				t.Errorf("Get(%q) = %v (%T), want %v (%T)",
					tt.path, got, got, tt.value, tt.value)
			}
		})
	}

	// Verify non-existent path returns ok=false
	t.Run("missing", func(t *testing.T) {
		_, ok := doc.Get("/nonexistent")
		if ok {
			t.Error("Get(/nonexistent) returned ok=true, want false")
		}
	})

	// Verify Get("") contains all values
	t.Run("root_contains_all", func(t *testing.T) {
		root, ok := doc.Get("")
		if !ok {
			t.Fatal("Get(\"\") returned ok=false")
		}

		rootMap, isMap := root.(map[string]any)
		if !isMap {
			t.Fatalf("Get(\"\") returned %T, want map[string]any", root)
		}

		// Check each value exists in root
		for _, tt := range tests {
			// Extract key from path (e.g., "/null_value" -> "null_value")
			key := tt.path[1:]
			val, exists := rootMap[key]
			if !exists {
				t.Errorf("root missing key %q", key)
				continue
			}

			if !valuesEqual(val, tt.value) {
				t.Errorf("root[%q] = %v (%T), want %v (%T)",
					key, val, val, tt.value, tt.value)
			}
		}
	})
}

// testArrayPaths verifies array/slice path operations work correctly.
func (dt *DocumentTester) testArrayPaths(t *testing.T) {
	doc := dt.newDocument()

	// Check if format supports arrays via MarshalTestData
	testData := map[string]any{"items": []any{"a", "b", "c"}}
	_, err := dt.parser.MarshalTestData(testData)
	if isUnsupportedError(err) {
		t.Skip("format does not support arrays: " + err.Error())
	}

	// Set array value
	arr := []any{"a", "b", "c"}
	if err := doc.Set("/items", arr); err != nil {
		t.Fatalf("Set(/items) error = %v", err)
	}

	// Get entire array
	got, ok := doc.Get("/items")
	if !ok {
		t.Fatal("Get(/items) returned ok=false")
	}

	gotArr, isArr := got.([]any)
	if !isArr {
		t.Fatalf("Get(/items) returned %T, want []any", got)
	}

	if len(gotArr) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(gotArr))
	}

	// Get array element by index
	elem, ok := doc.Get("/items/0")
	if !ok {
		t.Fatal("Get(/items/0) returned ok=false")
	}
	if elem != "a" {
		t.Errorf("Get(/items/0) = %v, want \"a\"", elem)
	}

	// Get root should contain array
	root, ok := doc.Get("")
	if !ok {
		t.Fatal("Get(\"\") returned ok=false")
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

// TestParser creates a Parser for testing from a document factory function.
// This is useful for testing document implementations that don't have a
// standard parser (e.g., internal document types).
//
// Example:
//
//	parser := jktest.TestParser(document.FormatYAML, func() document.Document {
//	    return myDocument{}
//	})
//	jktest.NewDocumentTester(t, parser).TestAll()
func TestParser(format document.DocumentFormat, factory func() document.Document) document.Parser {
	return &testParser{format: format, factory: factory}
}

// testParser implements document.Parser for testing purposes.
type testParser struct {
	format  document.DocumentFormat
	factory func() document.Document
}

func (p *testParser) Parse(data []byte) (document.Document, error) {
	return p.factory(), nil
}

func (p *testParser) Format() document.DocumentFormat {
	return p.format
}

func (p *testParser) CanMarshal() bool {
	return true
}

func (p *testParser) MarshalTestData(data map[string]any) ([]byte, error) {
	doc := p.factory()
	return doc.MarshalTestData(data)
}
