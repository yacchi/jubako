package jubako

import (
	"context"
	"reflect"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/format/json"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/types"
)

// mockWritableSource is a test source that supports saving
type mockWritableSource struct {
	data []byte
}

func (s *mockWritableSource) Type() source.SourceType { return "mock" }
func (s *mockWritableSource) FillDetails(d *types.Details) {
	d.Path = "/mock/path"
}
func (s *mockWritableSource) Load(_ context.Context) ([]byte, error) {
	b := make([]byte, len(s.data))
	copy(b, s.data)
	return b, nil
}
func (s *mockWritableSource) Save(_ context.Context, updateFunc source.UpdateFunc) error {
	newData, err := updateFunc(s.data)
	if err != nil {
		return err
	}
	s.data = newData
	return nil
}
func (s *mockWritableSource) CanSave() bool { return true }

// newMockWritableLayer creates a writable layer for testing
func newMockWritableLayer(name string, src source.Source) layer.Layer {
	return layer.New(layer.Name(name), src, json.New())
}

// mockWritableLayer wraps a Layer to make it implement the full interface
type mockWritableLayer struct {
	layer.Layer
}

func (l *mockWritableLayer) Watch(opts ...layer.WatchOption) (layer.LayerWatcher, error) {
	return layer.NewNoopLayerWatcher(), nil
}

func (l *mockWritableLayer) Save(ctx context.Context, changeset document.JSONPatchSet) error {
	return l.Layer.Save(ctx, changeset)
}

func TestDefaultValueConverter_SameType(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		targetType reflect.Type
	}{
		{"string", "hello", reflect.TypeOf("")},
		{"int", 42, reflect.TypeOf(0)},
		{"bool", true, reflect.TypeOf(false)},
		{"float64", 3.14, reflect.TypeOf(0.0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, tt.targetType)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.value {
				t.Errorf("expected %v, got %v", tt.value, result)
			}
		})
	}
}

func TestDefaultValueConverter_NilValue(t *testing.T) {
	result, err := DefaultValueConverter("/test", nil, reflect.TypeOf(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestDefaultValueConverter_ToBool(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected bool
		wantErr  bool
	}{
		// String conversions
		{"string true", "true", true, false},
		{"string TRUE", "TRUE", true, false},
		{"string True", "True", true, false},
		{"string 1", "1", true, false},
		{"string yes", "yes", true, false},
		{"string YES", "YES", true, false},
		{"string on", "on", true, false},
		{"string ON", "ON", true, false},
		{"string t", "t", true, false},
		{"string y", "y", true, false},
		{"string false", "false", false, false},
		{"string FALSE", "FALSE", false, false},
		{"string 0", "0", false, false},
		{"string no", "no", false, false},
		{"string off", "off", false, false},
		{"string f", "f", false, false},
		{"string n", "n", false, false},
		{"string empty", "", false, false},
		{"string with spaces", "  true  ", true, false},
		{"string invalid", "invalid", false, true},
		// Numeric conversions
		{"int 1", int(1), true, false},
		{"int 0", int(0), false, false},
		{"int -1", int(-1), true, false},
		{"int8 1", int8(1), true, false},
		{"int8 0", int8(0), false, false},
		{"int16 1", int16(1), true, false},
		{"int32 1", int32(1), true, false},
		{"int64 1", int64(1), true, false},
		{"uint 1", uint(1), true, false},
		{"uint 0", uint(0), false, false},
		{"uint8 1", uint8(1), true, false},
		{"uint16 1", uint16(1), true, false},
		{"uint32 1", uint32(1), true, false},
		{"uint64 1", uint64(1), true, false},
		{"float32 1.0", float32(1.0), true, false},
		{"float32 0.0", float32(0.0), false, false},
		{"float64 1.0", float64(1.0), true, false},
		{"float64 0.0", float64(0.0), false, false},
		// Bool (passthrough)
		{"bool true", true, true, false},
		{"bool false", false, false, false},
		// Unsupported types
		{"slice", []int{1, 2, 3}, false, true},
		{"map", map[string]int{"a": 1}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, reflect.TypeOf(false))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestDefaultValueConverter_ToInt(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		targetType reflect.Type
		expected   any
		wantErr    bool
	}{
		// String conversions
		{"string to int", "42", reflect.TypeOf(int(0)), int(42), false},
		{"string to int8", "42", reflect.TypeOf(int8(0)), int8(42), false},
		{"string to int16", "42", reflect.TypeOf(int16(0)), int16(42), false},
		{"string to int32", "42", reflect.TypeOf(int32(0)), int32(42), false},
		{"string to int64", "42", reflect.TypeOf(int64(0)), int64(42), false},
		{"string negative", "-42", reflect.TypeOf(int(0)), int(-42), false},
		{"string with spaces", "  42  ", reflect.TypeOf(int(0)), int(42), false},
		{"string hex", "0x2A", reflect.TypeOf(int(0)), int(42), false},
		{"string octal", "052", reflect.TypeOf(int(0)), int(42), false},
		{"string float", "3.14", reflect.TypeOf(int(0)), int(3), false},
		{"string invalid", "invalid", reflect.TypeOf(int(0)), nil, true},
		// Bool conversions
		{"bool true to int", true, reflect.TypeOf(int(0)), int(1), false},
		{"bool false to int", false, reflect.TypeOf(int(0)), int(0), false},
		// Numeric conversions
		{"int to int", int(42), reflect.TypeOf(int(0)), int(42), false},
		{"int8 to int", int8(42), reflect.TypeOf(int(0)), int(42), false},
		{"int16 to int", int16(42), reflect.TypeOf(int(0)), int(42), false},
		{"int32 to int", int32(42), reflect.TypeOf(int(0)), int(42), false},
		{"int64 to int", int64(42), reflect.TypeOf(int(0)), int(42), false},
		{"uint to int", uint(42), reflect.TypeOf(int(0)), int(42), false},
		{"uint8 to int", uint8(42), reflect.TypeOf(int(0)), int(42), false},
		{"uint16 to int", uint16(42), reflect.TypeOf(int(0)), int(42), false},
		{"uint32 to int", uint32(42), reflect.TypeOf(int(0)), int(42), false},
		{"uint64 to int", uint64(42), reflect.TypeOf(int(0)), int(42), false},
		{"float32 to int", float32(42.5), reflect.TypeOf(int(0)), int(42), false},
		{"float64 to int", float64(42.9), reflect.TypeOf(int(0)), int(42), false},
		// Unsupported
		{"slice to int", []int{1}, reflect.TypeOf(int(0)), nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, tt.targetType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestDefaultValueConverter_ToUint(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		targetType reflect.Type
		expected   any
		wantErr    bool
	}{
		// String conversions
		{"string to uint", "42", reflect.TypeOf(uint(0)), uint(42), false},
		{"string to uint8", "42", reflect.TypeOf(uint8(0)), uint8(42), false},
		{"string to uint16", "42", reflect.TypeOf(uint16(0)), uint16(42), false},
		{"string to uint32", "42", reflect.TypeOf(uint32(0)), uint32(42), false},
		{"string to uint64", "42", reflect.TypeOf(uint64(0)), uint64(42), false},
		{"string with spaces", "  42  ", reflect.TypeOf(uint(0)), uint(42), false},
		{"string float", "3.14", reflect.TypeOf(uint(0)), uint(3), false},
		{"string negative", "-1", reflect.TypeOf(uint(0)), nil, true},
		{"string invalid", "invalid", reflect.TypeOf(uint(0)), nil, true},
		// Bool conversions
		{"bool true to uint", true, reflect.TypeOf(uint(0)), uint(1), false},
		{"bool false to uint", false, reflect.TypeOf(uint(0)), uint(0), false},
		// Numeric conversions
		{"int to uint", int(42), reflect.TypeOf(uint(0)), uint(42), false},
		{"int8 to uint", int8(42), reflect.TypeOf(uint(0)), uint(42), false},
		{"int16 to uint", int16(42), reflect.TypeOf(uint(0)), uint(42), false},
		{"int32 to uint", int32(42), reflect.TypeOf(uint(0)), uint(42), false},
		{"int64 to uint", int64(42), reflect.TypeOf(uint(0)), uint(42), false},
		{"uint to uint", uint(42), reflect.TypeOf(uint(0)), uint(42), false},
		{"uint8 to uint", uint8(42), reflect.TypeOf(uint(0)), uint(42), false},
		{"uint16 to uint", uint16(42), reflect.TypeOf(uint(0)), uint(42), false},
		{"uint32 to uint", uint32(42), reflect.TypeOf(uint(0)), uint(42), false},
		{"uint64 to uint", uint64(42), reflect.TypeOf(uint(0)), uint(42), false},
		{"float32 to uint", float32(42.5), reflect.TypeOf(uint(0)), uint(42), false},
		{"float64 to uint", float64(42.9), reflect.TypeOf(uint(0)), uint(42), false},
		// Unsupported
		{"slice to uint", []int{1}, reflect.TypeOf(uint(0)), nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, tt.targetType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestDefaultValueConverter_ToFloat(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		targetType reflect.Type
		expected   any
		wantErr    bool
	}{
		// String conversions
		{"string to float32", "3.14", reflect.TypeOf(float32(0)), float32(3.14), false},
		{"string to float64", "3.14", reflect.TypeOf(float64(0)), float64(3.14), false},
		{"string int to float64", "42", reflect.TypeOf(float64(0)), float64(42), false},
		{"string negative", "-3.14", reflect.TypeOf(float64(0)), float64(-3.14), false},
		{"string with spaces", "  3.14  ", reflect.TypeOf(float64(0)), float64(3.14), false},
		{"string invalid", "invalid", reflect.TypeOf(float64(0)), nil, true},
		// Bool conversions
		{"bool true to float64", true, reflect.TypeOf(float64(0)), float64(1), false},
		{"bool false to float64", false, reflect.TypeOf(float64(0)), float64(0), false},
		// Numeric conversions
		{"int to float64", int(42), reflect.TypeOf(float64(0)), float64(42), false},
		{"int8 to float64", int8(42), reflect.TypeOf(float64(0)), float64(42), false},
		{"int16 to float64", int16(42), reflect.TypeOf(float64(0)), float64(42), false},
		{"int32 to float64", int32(42), reflect.TypeOf(float64(0)), float64(42), false},
		{"int64 to float64", int64(42), reflect.TypeOf(float64(0)), float64(42), false},
		{"uint to float64", uint(42), reflect.TypeOf(float64(0)), float64(42), false},
		{"uint8 to float64", uint8(42), reflect.TypeOf(float64(0)), float64(42), false},
		{"uint16 to float64", uint16(42), reflect.TypeOf(float64(0)), float64(42), false},
		{"uint32 to float64", uint32(42), reflect.TypeOf(float64(0)), float64(42), false},
		{"uint64 to float64", uint64(42), reflect.TypeOf(float64(0)), float64(42), false},
		{"float32 to float64", float32(3.14), reflect.TypeOf(float64(0)), float64(float32(3.14)), false},
		{"float64 to float64", float64(3.14), reflect.TypeOf(float64(0)), float64(3.14), false},
		// Unsupported
		{"slice to float64", []int{1}, reflect.TypeOf(float64(0)), nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, tt.targetType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestDefaultValueConverter_ToString(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"string passthrough", "hello", "hello"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"int", int(42), "42"},
		{"int8", int8(42), "42"},
		{"int16", int16(42), "42"},
		{"int32", int32(42), "42"},
		{"int64", int64(42), "42"},
		{"uint", uint(42), "42"},
		{"uint8", uint8(42), "42"},
		{"uint16", uint16(42), "42"},
		{"uint32", uint32(42), "42"},
		{"uint64", uint64(42), "42"},
		{"float32", float32(3.14), "3.14"},
		{"float64", float64(3.14), "3.14"},
		{"other type", struct{ Name string }{Name: "test"}, "{test}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, reflect.TypeOf(""))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDefaultValueConverter_ToSlice(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		targetType reflect.Type
		expected   any
		wantErr    bool
	}{
		{
			name:       "[]any to []int",
			value:      []any{1, 2, 3},
			targetType: reflect.TypeOf([]int{}),
			expected:   []int{1, 2, 3},
			wantErr:    false,
		},
		{
			name:       "[]any with strings to []int",
			value:      []any{"1", "2", "3"},
			targetType: reflect.TypeOf([]int{}),
			expected:   []int{1, 2, 3},
			wantErr:    false,
		},
		{
			name:       "[]any to []string",
			value:      []any{"a", "b", "c"},
			targetType: reflect.TypeOf([]string{}),
			expected:   []string{"a", "b", "c"},
			wantErr:    false,
		},
		{
			name:       "[]any with ints to []string",
			value:      []any{1, 2, 3},
			targetType: reflect.TypeOf([]string{}),
			expected:   []string{"1", "2", "3"},
			wantErr:    false,
		},
		{
			name:       "[]any to []bool",
			value:      []any{"true", "false", "1", "0"},
			targetType: reflect.TypeOf([]bool{}),
			expected:   []bool{true, false, true, false},
			wantErr:    false,
		},
		{
			name:       "[]int to []int (via reflection)",
			value:      []int{1, 2, 3},
			targetType: reflect.TypeOf([]int{}),
			expected:   []int{1, 2, 3},
			wantErr:    false,
		},
		{
			name:       "[]any with invalid element",
			value:      []any{"1", "invalid", "3"},
			targetType: reflect.TypeOf([]int{}),
			expected:   nil,
			wantErr:    true,
		},
		{
			name:       "non-slice value",
			value:      "not a slice",
			targetType: reflect.TypeOf([]int{}),
			expected:   "not a slice",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, tt.targetType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestDefaultValueConverter_ToMap(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		targetType reflect.Type
		expected   any
		wantErr    bool
	}{
		{
			name:       "map[string]any to map[string]int",
			value:      map[string]any{"a": 1, "b": 2},
			targetType: reflect.TypeOf(map[string]int{}),
			expected:   map[string]int{"a": 1, "b": 2},
			wantErr:    false,
		},
		{
			name:       "map[string]any with strings to map[string]int",
			value:      map[string]any{"a": "1", "b": "2"},
			targetType: reflect.TypeOf(map[string]int{}),
			expected:   map[string]int{"a": 1, "b": 2},
			wantErr:    false,
		},
		{
			name:       "map[string]any to map[string]string",
			value:      map[string]any{"a": 1, "b": 2},
			targetType: reflect.TypeOf(map[string]string{}),
			expected:   map[string]string{"a": "1", "b": "2"},
			wantErr:    false,
		},
		{
			name:       "map[string]any with invalid value",
			value:      map[string]any{"a": "1", "b": "invalid"},
			targetType: reflect.TypeOf(map[string]int{}),
			expected:   nil,
			wantErr:    true,
		},
		{
			name:       "non-map value",
			value:      "not a map",
			targetType: reflect.TypeOf(map[string]int{}),
			expected:   "not a map",
			wantErr:    false,
		},
		{
			name:       "map with non-string key (unsupported)",
			value:      map[string]any{"a": 1},
			targetType: reflect.TypeOf(map[int]int{}),
			expected:   map[string]any{"a": 1},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, tt.targetType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestDefaultValueConverter_ToPointer(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		targetType reflect.Type
		expected   any
	}{
		{
			name:       "string to *int",
			value:      "42",
			targetType: reflect.TypeOf((*int)(nil)),
			expected:   intPtr(42),
		},
		{
			name:       "string to *bool",
			value:      "true",
			targetType: reflect.TypeOf((*bool)(nil)),
			expected:   boolPtr(true),
		},
		{
			name:       "string to *string",
			value:      "hello",
			targetType: reflect.TypeOf((*string)(nil)),
			expected:   stringPtr("hello"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, tt.targetType)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestDefaultValueConverter_UnsupportedTarget(t *testing.T) {
	// When target type is unsupported, value should be returned as-is
	tests := []struct {
		name       string
		value      any
		targetType reflect.Type
	}{
		{"struct target", "hello", reflect.TypeOf(struct{}{})},
		{"chan target", "hello", reflect.TypeOf(make(chan int))},
		{"func target", "hello", reflect.TypeOf(func() {})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, tt.targetType)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.value {
				t.Errorf("expected value to be returned as-is: %v, got %v", tt.value, result)
			}
		})
	}
}

func TestDefaultValueConverter_ToSlice_EdgeCases(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result, err := DefaultValueConverter("/test", []any{}, reflect.TypeOf([]int{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{}
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("slice with nil elements", func(t *testing.T) {
		result, err := DefaultValueConverter("/test", []any{1, nil, 3}, reflect.TypeOf([]int{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{1, 0, 3}
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("non-slice non-any", func(t *testing.T) {
		// A string is not a slice, should be returned as-is
		result, err := DefaultValueConverter("/test", 123, reflect.TypeOf([]int{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != 123 {
			t.Errorf("expected 123, got %v", result)
		}
	})
}

func TestDefaultValueConverter_ToMap_EdgeCases(t *testing.T) {
	t.Run("empty map", func(t *testing.T) {
		result, err := DefaultValueConverter("/test", map[string]any{}, reflect.TypeOf(map[string]int{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := map[string]int{}
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("map with nil values", func(t *testing.T) {
		result, err := DefaultValueConverter("/test", map[string]any{"a": 1, "b": nil}, reflect.TypeOf(map[string]int{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := map[string]int{"a": 1}
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})
}

func TestDefaultValueConverter_Interface(t *testing.T) {
	// Test that we can use a custom converter
	customConverter := func(path string, value any, targetType reflect.Type) (any, error) {
		// Custom: always return 42 for ints
		if targetType.Kind() == reflect.Int {
			return 42, nil
		}
		return DefaultValueConverter(path, value, targetType)
	}

	result, err := customConverter("/test", "100", reflect.TypeOf(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %v", result)
	}

	// Fall back to default for other types
	result, err = customConverter("/test", "true", reflect.TypeOf(false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != true {
		t.Errorf("expected true, got %v", result)
	}
}

// Additional tests for 100% coverage

func TestConvertToBool_AllBranches(t *testing.T) {
	// Test int16, int32, int64 branches
	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{"int16 non-zero", int16(5), true},
		{"int32 non-zero", int32(5), true},
		{"int64 non-zero", int64(5), true},
		{"uint16 non-zero", uint16(5), true},
		{"uint32 non-zero", uint32(5), true},
		{"uint64 non-zero", uint64(5), true},
		{"float32 non-zero", float32(0.5), true},
		{"float64 non-zero", float64(0.5), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, reflect.TypeOf(false))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConvertToInt_AllBranches(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		targetType reflect.Type
		expected   any
	}{
		{"int8 source", int8(10), reflect.TypeOf(int64(0)), int64(10)},
		{"int16 source", int16(10), reflect.TypeOf(int64(0)), int64(10)},
		{"int32 source", int32(10), reflect.TypeOf(int64(0)), int64(10)},
		{"uint8 source", uint8(10), reflect.TypeOf(int64(0)), int64(10)},
		{"uint16 source", uint16(10), reflect.TypeOf(int64(0)), int64(10)},
		{"uint32 source", uint32(10), reflect.TypeOf(int64(0)), int64(10)},
		{"uint64 source", uint64(10), reflect.TypeOf(int64(0)), int64(10)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, tt.targetType)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestConvertToUint_AllBranches(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		targetType reflect.Type
		expected   any
	}{
		{"int8 source", int8(10), reflect.TypeOf(uint64(0)), uint64(10)},
		{"int16 source", int16(10), reflect.TypeOf(uint64(0)), uint64(10)},
		{"int32 source", int32(10), reflect.TypeOf(uint64(0)), uint64(10)},
		{"uint8 source", uint8(10), reflect.TypeOf(uint64(0)), uint64(10)},
		{"uint16 source", uint16(10), reflect.TypeOf(uint64(0)), uint64(10)},
		{"uint32 source", uint32(10), reflect.TypeOf(uint64(0)), uint64(10)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, tt.targetType)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestConvertToFloat_AllBranches(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		targetType reflect.Type
		expected   any
	}{
		{"int8 source", int8(10), reflect.TypeOf(float64(0)), float64(10)},
		{"int16 source", int16(10), reflect.TypeOf(float64(0)), float64(10)},
		{"int32 source", int32(10), reflect.TypeOf(float64(0)), float64(10)},
		{"uint8 source", uint8(10), reflect.TypeOf(float64(0)), float64(10)},
		{"uint16 source", uint16(10), reflect.TypeOf(float64(0)), float64(10)},
		{"uint32 source", uint32(10), reflect.TypeOf(float64(0)), float64(10)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, tt.targetType)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestConvertToString_AllBranches(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"int8", int8(42), "42"},
		{"int16", int16(42), "42"},
		{"int32", int32(42), "42"},
		{"uint8", uint8(42), "42"},
		{"uint16", uint16(42), "42"},
		{"uint32", uint32(42), "42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultValueConverter("/test", tt.value, reflect.TypeOf(""))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestConvertToSlice_AllBranches(t *testing.T) {
	t.Run("typed slice via reflection", func(t *testing.T) {
		// A typed slice (not []any) should be converted via reflection
		result, err := DefaultValueConverter("/test", []string{"1", "2", "3"}, reflect.TypeOf([]int{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{1, 2, 3}
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})
}

func TestConvertToPointer_Error(t *testing.T) {
	// Test pointer conversion with an error
	_, err := DefaultValueConverter("/test", "invalid", reflect.TypeOf((*int)(nil)))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// Integration test with Store.SetTo
func TestStore_SetTo_WithTypeConversion(t *testing.T) {
	type Config struct {
		Port    int    `json:"port"`
		Enabled bool   `json:"enabled"`
		Name    string `json:"name"`
		Rate    float64 `json:"rate"`
	}

	store := New[Config]()

	// Add a writable layer
	src := &mockWritableSource{data: []byte(`{"port": 8080, "enabled": true, "name": "test", "rate": 1.5}`)}
	l := newMockWritableLayer("test", src)
	err := store.Add(l)
	if err != nil {
		t.Fatalf("failed to add layer: %v", err)
	}

	err = store.Load(t.Context())
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// Test string to int conversion
	err = store.SetTo("test", "/port", "9000")
	if err != nil {
		t.Fatalf("failed to SetTo with string->int: %v", err)
	}
	cfg := store.Get()
	if cfg.Port != 9000 {
		t.Errorf("expected port=9000, got %d", cfg.Port)
	}

	// Test string to bool conversion
	err = store.SetTo("test", "/enabled", "false")
	if err != nil {
		t.Fatalf("failed to SetTo with string->bool: %v", err)
	}
	cfg = store.Get()
	if cfg.Enabled != false {
		t.Errorf("expected enabled=false, got %v", cfg.Enabled)
	}

	// Test int to string conversion
	err = store.SetTo("test", "/name", 12345)
	if err != nil {
		t.Fatalf("failed to SetTo with int->string: %v", err)
	}
	cfg = store.Get()
	if cfg.Name != "12345" {
		t.Errorf("expected name=\"12345\", got %q", cfg.Name)
	}

	// Test string to float conversion
	err = store.SetTo("test", "/rate", "2.5")
	if err != nil {
		t.Fatalf("failed to SetTo with string->float: %v", err)
	}
	cfg = store.Get()
	if cfg.Rate != 2.5 {
		t.Errorf("expected rate=2.5, got %f", cfg.Rate)
	}
}

func TestStore_SetTo_ConversionError(t *testing.T) {
	type Config struct {
		Port int `json:"port"`
	}

	store := New[Config]()

	src := &mockWritableSource{data: []byte(`{"port": 8080}`)}
	l := newMockWritableLayer("test", src)
	err := store.Add(l)
	if err != nil {
		t.Fatalf("failed to add layer: %v", err)
	}

	err = store.Load(t.Context())
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// Test conversion error
	err = store.SetTo("test", "/port", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid conversion, got nil")
	}
}

func TestStore_WithValueConverter_Custom(t *testing.T) {
	type Config struct {
		Value int `json:"value"`
	}

	// Custom converter that always returns 42 for ints
	customConverter := func(path string, value any, targetType reflect.Type) (any, error) {
		if targetType.Kind() == reflect.Int {
			return 42, nil
		}
		return DefaultValueConverter(path, value, targetType)
	}

	store := New[Config](WithValueConverter(customConverter))

	src := &mockWritableSource{data: []byte(`{"value": 0}`)}
	l := newMockWritableLayer("test", src)
	err := store.Add(l)
	if err != nil {
		t.Fatalf("failed to add layer: %v", err)
	}

	err = store.Load(t.Context())
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// Set with string, should be converted to 42 by custom converter
	err = store.SetTo("test", "/value", "100")
	if err != nil {
		t.Fatalf("failed to SetTo: %v", err)
	}

	cfg := store.Get()
	if cfg.Value != 42 {
		t.Errorf("expected value=42 from custom converter, got %d", cfg.Value)
	}
}

// Helper functions for pointer creation
func intPtr(v int) *int          { return &v }
func boolPtr(v bool) *bool       { return &v }
func stringPtr(v string) *string { return &v }
