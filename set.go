package jubako

import (
	"reflect"

	"github.com/yacchi/jubako/jsonptr"
)

// SetOption is a functional option for configuring Set method behavior.
// It can specify values to set (String, Int, etc.) or modify behavior (SkipZeroValues, etc.).
type SetOption func(*setConfig)

// setConfig holds the configuration for Set method.
type setConfig struct {
	patches        []pathValue // path-value pairs to set
	skipZeroValues bool        // skip zero-value entries
	deleteNilValue bool        // treat nil values as delete operations
}

// pathValue represents a single path-value pair to set.
type pathValue struct {
	path  string
	value any
}

// String specifies a string value to set at the given path.
func String(path string, value string) SetOption {
	return func(c *setConfig) {
		c.patches = append(c.patches, pathValue{path: path, value: value})
	}
}

// Int specifies an integer value to set at the given path.
func Int(path string, value int) SetOption {
	return func(c *setConfig) {
		c.patches = append(c.patches, pathValue{path: path, value: value})
	}
}

// Int64 specifies an int64 value to set at the given path.
func Int64(path string, value int64) SetOption {
	return func(c *setConfig) {
		c.patches = append(c.patches, pathValue{path: path, value: value})
	}
}

// Float specifies a float64 value to set at the given path.
func Float(path string, value float64) SetOption {
	return func(c *setConfig) {
		c.patches = append(c.patches, pathValue{path: path, value: value})
	}
}

// Bool specifies a boolean value to set at the given path.
func Bool(path string, value bool) SetOption {
	return func(c *setConfig) {
		c.patches = append(c.patches, pathValue{path: path, value: value})
	}
}

// Value specifies a value of any type to set at the given path.
// Use this for types not covered by the typed functions (String, Int, etc.).
func Value(path string, value any) SetOption {
	return func(c *setConfig) {
		c.patches = append(c.patches, pathValue{path: path, value: value})
	}
}

// Path groups multiple SetOptions under a common path prefix.
// Child options use relative paths that are joined with the prefix.
//
// Example:
//
//	jubako.Path("/server",
//	    jubako.Int("port", 8080),      // → /server/port
//	    jubako.String("host", "localhost"), // → /server/host
//	)
func Path(prefix string, opts ...SetOption) SetOption {
	return func(c *setConfig) {
		// Create a temporary config to collect child patches
		childConfig := &setConfig{}
		for _, opt := range opts {
			opt(childConfig)
		}

		// Add prefix to all child patches
		for _, pv := range childConfig.patches {
			fullPath := jsonptr.Join(prefix, pv.path)
			c.patches = append(c.patches, pathValue{path: fullPath, value: pv.value})
		}
	}
}

// Map expands a map into multiple path-value pairs at the given base path.
// Map keys are used as relative paths under the base path.
//
// Example:
//
//	jubako.Map("/settings", map[string]any{
//	    "port": 8080,        // → /settings/port
//	    "host": "localhost", // → /settings/host
//	})
func Map(path string, m map[string]any) SetOption {
	return func(c *setConfig) {
		for key, value := range m {
			fullPath := jsonptr.Join(path, key)
			c.patches = append(c.patches, pathValue{path: fullPath, value: value})
		}
	}
}

// Struct expands a struct into multiple path-value pairs at the given base path.
// Field names are derived from json tags, falling back to field names.
// Only exported fields are included.
//
// Example:
//
//	type Credential struct {
//	    Username string `json:"username"`
//	    Password string `json:"password"`
//	}
//	jubako.Struct("/credential", cred)
//	// → /credential/username, /credential/password
func Struct(path string, v any) SetOption {
	return func(c *setConfig) {
		patches := expandStruct(path, v)
		c.patches = append(c.patches, patches...)
	}
}

// SkipZeroValues configures Set to skip entries with zero values.
// This applies to all value types: empty strings, 0 for numbers, false for bools,
// nil for pointers/slices/maps, and zero-valued structs.
func SkipZeroValues() SetOption {
	return func(c *setConfig) {
		c.skipZeroValues = true
	}
}

// DeleteNilValue configures Set to treat nil values as delete operations.
// When enabled, paths with nil values will be removed from the layer
// instead of being set to nil.
func DeleteNilValue() SetOption {
	return func(c *setConfig) {
		c.deleteNilValue = true
	}
}

// expandStruct converts a struct to path-value pairs using reflection.
func expandStruct(basePath string, v any) []pathValue {
	var patches []pathValue

	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return patches
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		// Not a struct, treat as single value
		patches = append(patches, pathValue{path: basePath, value: v})
		return patches
	}

	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get field name from json tag or use field name
		fieldName := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
			// Parse json tag (handle "name,omitempty" format)
			for j := 0; j < len(jsonTag); j++ {
				if jsonTag[j] == ',' {
					jsonTag = jsonTag[:j]
					break
				}
			}
			if jsonTag != "" {
				fieldName = jsonTag
			}
		}

		fieldPath := jsonptr.Join(basePath, fieldName)

		// Handle nested structs recursively, but treat leaf types (like time.Time) as values
		if fieldVal.Kind() == reflect.Struct && !isLeafType(fieldVal.Type()) {
			nestedPatches := expandStruct(fieldPath, fieldVal.Interface())
			patches = append(patches, nestedPatches...)
		} else if fieldVal.Kind() == reflect.Ptr && !fieldVal.IsNil() && fieldVal.Elem().Kind() == reflect.Struct && !isLeafType(fieldVal.Elem().Type()) {
			nestedPatches := expandStruct(fieldPath, fieldVal.Interface())
			patches = append(patches, nestedPatches...)
		} else {
			patches = append(patches, pathValue{path: fieldPath, value: fieldVal.Interface()})
		}
	}

	return patches
}

// isLeafType checks if a type should be treated as a leaf value rather than recursively expanded.
// A struct is considered a leaf type if it has no exported fields (like time.Time, net.IP, etc.).
func isLeafType(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	// Check if the struct has any exported fields
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).IsExported() {
			return false
		}
	}
	// No exported fields - treat as leaf value
	return true
}

// isZeroValue checks if a value is the zero value for its type.
func isZeroValue(v any) bool {
	if v == nil {
		return true
	}
	val := reflect.ValueOf(v)
	return val.IsZero()
}
