// Package container provides utility functions for deep copying Go container types
// (maps and slices) commonly used in configuration data structures.
//
// These utilities are useful for format and source implementations that need to
// safely copy nested map[string]any and []any structures.
package container

// DeepCopyMap creates a deep copy of a map[string]any, recursively copying
// nested maps and slices.
func DeepCopyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}

	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = DeepCopyValue(v)
	}
	return dst
}

// DeepCopySlice creates a deep copy of a []any, recursively copying
// nested maps and slices.
func DeepCopySlice(src []any) []any {
	if src == nil {
		return nil
	}

	dst := make([]any, len(src))
	for i, v := range src {
		dst[i] = DeepCopyValue(v)
	}
	return dst
}

// DeepCopyValue creates a deep copy of a value. For map[string]any and []any,
// it recursively copies the contents. Other types are returned as-is.
func DeepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return DeepCopyMap(val)
	case []any:
		return DeepCopySlice(val)
	default:
		return v
	}
}
