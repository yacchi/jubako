package container

import (
	"testing"
)

func TestDeepCopyMap(t *testing.T) {
	original := map[string]any{
		"a": 1,
		"nested": map[string]any{
			"b": 2,
		},
		"items": []any{"x", "y"},
	}

	copied := DeepCopyMap(original)

	// Modify copy
	copied["a"] = 100
	copied["nested"].(map[string]any)["b"] = 200
	copied["items"].([]any)[0] = "modified"

	// Original should be unchanged
	if original["a"] != 1 {
		t.Error("original[a] was modified")
	}
	if original["nested"].(map[string]any)["b"] != 2 {
		t.Error("original[nested][b] was modified")
	}
	if original["items"].([]any)[0] != "x" {
		t.Error("original[items][0] was modified")
	}
}

func TestDeepCopyMap_Nil(t *testing.T) {
	if DeepCopyMap(nil) != nil {
		t.Error("DeepCopyMap(nil) should return nil")
	}
}

func TestDeepCopySlice(t *testing.T) {
	original := []any{
		"a",
		map[string]any{"key": "value"},
		[]any{1, 2, 3},
	}

	copied := DeepCopySlice(original)

	// Modify copy
	copied[0] = "modified"
	copied[1].(map[string]any)["key"] = "modified"
	copied[2].([]any)[0] = 100

	// Original should be unchanged
	if original[0] != "a" {
		t.Error("original[0] was modified")
	}
	if original[1].(map[string]any)["key"] != "value" {
		t.Error("original[1][key] was modified")
	}
	if original[2].([]any)[0] != 1 {
		t.Error("original[2][0] was modified")
	}
}

func TestDeepCopySlice_Nil(t *testing.T) {
	if DeepCopySlice(nil) != nil {
		t.Error("DeepCopySlice(nil) should return nil")
	}
}

func TestDeepCopyValue(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{"nil", nil},
		{"string", "hello"},
		{"int", 42},
		{"map", map[string]any{"key": "value"}},
		{"slice", []any{1, 2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := DeepCopyValue(tt.input)
			if tt.input == nil {
				if copied != nil {
					t.Error("DeepCopyValue(nil) should return nil")
				}
				return
			}

			// For maps and slices, verify independence
			switch v := tt.input.(type) {
			case map[string]any:
				copiedMap := copied.(map[string]any)
				copiedMap["new"] = "added"
				if _, ok := v["new"]; ok {
					t.Error("original map was modified")
				}
			case []any:
				copiedSlice := copied.([]any)
				copiedSlice[0] = "modified"
				if v[0] == "modified" {
					t.Error("original slice was modified")
				}
			}
		})
	}
}
