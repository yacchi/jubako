package jubako

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/yacchi/jubako/decoder"
	"github.com/yacchi/jubako/maputil"
)

const tagName = "jubako"

// MapDecoder is a function that decodes a map[string]any into a target struct.
// See decoder.Func for the function signature.
// The default implementation is decoder.JSON.
type MapDecoder = decoder.Func

// PathMapping represents a single field's path mapping from a jubako struct tag.
type PathMapping struct {
	// FieldKey is the JSON key used for decoding (from json tag or field name).
	FieldKey string
	// SourcePath is the JSONPointer path to retrieve the value from (from jubako tag).
	// Empty if this field has no jubako tag or is skipped.
	SourcePath string
	// Skipped is true if the field has jubako:"-" tag.
	Skipped bool
	// IsRelative is true if the path is relative to current context (no leading "/").
	// Relative paths are resolved from the current element context (e.g., in slices/maps).
	// Absolute paths (starting with "/") are resolved from the root.
	IsRelative bool
}

// MappingTable holds all path mappings for a struct type.
// Built once at Store initialization, used during every materialize.
type MappingTable struct {
	// Mappings contains direct field mappings at this level.
	Mappings []PathMapping
	// Nested contains mapping tables for nested struct fields (key = JSON key).
	Nested map[string]*MappingTable
	// SliceElement contains mapping table for slice element type (if slice of structs).
	SliceElement map[string]*MappingTable
	// MapValue contains mapping table for map value type (if map with struct values).
	MapValue map[string]*MappingTable
}

// buildMappingTable creates a mapping table for the given struct type.
// This is called once during Store initialization.
func buildMappingTable(t reflect.Type) *MappingTable {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	table := &MappingTable{
		Mappings:     make([]PathMapping, 0),
		Nested:       make(map[string]*MappingTable),
		SliceElement: make(map[string]*MappingTable),
		MapValue:     make(map[string]*MappingTable),
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		jsonKey := getJSONKey(field)
		if jsonKey == "-" {
			continue
		}

		// Check for jubako tag
		tag, hasTag := field.Tag.Lookup(tagName)
		if hasTag {
			if tag == "-" {
				table.Mappings = append(table.Mappings, PathMapping{
					FieldKey: jsonKey,
					Skipped:  true,
				})
			} else {
				path, isRelative := parseJubakoTag(tag)
				if path != "" {
					table.Mappings = append(table.Mappings, PathMapping{
						FieldKey:   jsonKey,
						SourcePath: path,
						IsRelative: isRelative,
					})
				}
			}
		}

		// Check for nested types (struct, slice, map) that may have jubako tags
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		switch fieldType.Kind() {
		case reflect.Struct:
			// Recursively build mapping table for nested struct
			if nested := buildMappingTable(fieldType); nested != nil && !nested.IsEmpty() {
				table.Nested[jsonKey] = nested
			}

		case reflect.Slice, reflect.Array:
			// Check if slice element is a struct with mappings
			elemType := fieldType.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct {
				if elemTable := buildMappingTable(elemType); elemTable != nil && !elemTable.IsEmpty() {
					table.SliceElement[jsonKey] = elemTable
				}
			}

		case reflect.Map:
			// Check if map value is a struct with mappings
			valueType := fieldType.Elem()
			if valueType.Kind() == reflect.Ptr {
				valueType = valueType.Elem()
			}
			if valueType.Kind() == reflect.Struct {
				if valueTable := buildMappingTable(valueType); valueTable != nil && !valueTable.IsEmpty() {
					table.MapValue[jsonKey] = valueTable
				}
			}
		}
	}

	return table
}

// String returns a human-readable representation of the mapping table.
func (t *MappingTable) String() string {
	if t == nil {
		return "(no mappings)"
	}
	var sb strings.Builder
	t.writeString(&sb, "")
	return sb.String()
}

func (t *MappingTable) writeString(sb *strings.Builder, indent string) {
	for _, m := range t.Mappings {
		if m.Skipped {
			fmt.Fprintf(sb, "%s%s: (skipped)\n", indent, m.FieldKey)
		} else if m.IsRelative {
			fmt.Fprintf(sb, "%s%s <- .%s (relative)\n", indent, m.FieldKey, m.SourcePath)
		} else {
			fmt.Fprintf(sb, "%s%s <- %s\n", indent, m.FieldKey, m.SourcePath)
		}
	}
	for key, nested := range t.Nested {
		fmt.Fprintf(sb, "%s%s:\n", indent, key)
		nested.writeString(sb, indent+"  ")
	}
	for key, elemTable := range t.SliceElement {
		fmt.Fprintf(sb, "%s%s[]: (slice element)\n", indent, key)
		elemTable.writeString(sb, indent+"  ")
	}
	for key, valueTable := range t.MapValue {
		fmt.Fprintf(sb, "%s%s{}: (map value)\n", indent, key)
		valueTable.writeString(sb, indent+"  ")
	}
}

// IsEmpty returns true if there are no mappings defined.
func (t *MappingTable) IsEmpty() bool {
	if t == nil {
		return true
	}
	return len(t.Mappings) == 0 && len(t.Nested) == 0 && len(t.SliceElement) == 0 && len(t.MapValue) == 0
}

// applyMappings applies the mapping table to transform the source map.
// Returns a new map with values remapped according to jubako tags.
func applyMappings(src map[string]any, table *MappingTable) map[string]any {
	return applyMappingsWithRoot(src, src, table)
}

// applyMappingsWithRoot applies the mapping table with access to the root source map.
// root is the original source map (for absolute path lookups in jubako tags).
// src is the current context map (for regular JSON field mappings and relative path lookups).
func applyMappingsWithRoot(root, src map[string]any, table *MappingTable) map[string]any {
	if table == nil {
		return src
	}

	// Start with a deep copy of the source (or empty map if src doesn't exist)
	var dst map[string]any
	if src != nil {
		dst = deepCopyMap(src)
	} else {
		dst = make(map[string]any)
	}

	// Apply direct mappings
	for _, m := range table.Mappings {
		if m.Skipped {
			// Remove this key from destination
			delete(dst, m.FieldKey)
		} else if m.SourcePath != "" {
			// Choose source based on path type
			var lookupSrc map[string]any
			if m.IsRelative {
				lookupSrc = src // Relative paths use current context
			} else {
				lookupSrc = root // Absolute paths use root
			}
			if value, ok := maputil.GetPath(lookupSrc, m.SourcePath); ok {
				dst[m.FieldKey] = value
			}
		}
	}

	// Apply nested struct mappings
	for fieldKey, nestedTable := range table.Nested {
		// Get sub-map from current src for regular JSON mappings
		subSrc, _ := src[fieldKey].(map[string]any)
		// Pass root for absolute path lookups, subSrc for JSON field context
		dst[fieldKey] = applyMappingsWithRoot(root, subSrc, nestedTable)
	}

	// Apply slice element mappings
	for fieldKey, elemTable := range table.SliceElement {
		if slice, ok := dst[fieldKey].([]any); ok {
			newSlice := make([]any, len(slice))
			for i, elem := range slice {
				if elemMap, ok := elem.(map[string]any); ok {
					// For each element, the element itself becomes the context for relative paths
					newSlice[i] = applyMappingsWithRoot(root, elemMap, elemTable)
				} else {
					newSlice[i] = elem
				}
			}
			dst[fieldKey] = newSlice
		}
	}

	// Apply map value mappings
	for fieldKey, valueTable := range table.MapValue {
		if m, ok := dst[fieldKey].(map[string]any); ok {
			newMap := make(map[string]any, len(m))
			for k, v := range m {
				if valueMap, ok := v.(map[string]any); ok {
					// For each value, the value itself becomes the context for relative paths
					newMap[k] = applyMappingsWithRoot(root, valueMap, valueTable)
				} else {
					newMap[k] = v
				}
			}
			dst[fieldKey] = newMap
		}
	}

	return dst
}

// getJSONKey returns the JSON key for a struct field.
// Uses the json tag if present, otherwise the field name.
func getJSONKey(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name
	}
	key := parseJSONTagKey(tag)
	if key == "" {
		return field.Name
	}
	return key
}

// parseJubakoTag extracts the path and relative flag from a jubako tag.
//
// Path formats:
//   - "/path/to/value" - absolute path from root
//   - "path/to/value"  - relative path from current context
//   - "./path/to/value" - relative path (explicit, "./" is stripped)
//
// Options can be appended after comma (future: omitempty, default, etc.)
func parseJubakoTag(tag string) (path string, isRelative bool) {
	// Handle options (future: omitempty, default, etc.)
	if idx := strings.Index(tag, ","); idx >= 0 {
		tag = tag[:idx]
	}

	// Check for explicit relative prefix
	if strings.HasPrefix(tag, "./") {
		return "/" + tag[2:], true // Convert "./foo" to "/foo" for JSONPointer
	}

	// Absolute paths start with "/"
	if strings.HasPrefix(tag, "/") {
		return tag, false
	}

	// No leading "/" means relative path
	if tag != "" {
		return "/" + tag, true // Prepend "/" for JSONPointer format
	}

	return tag, false
}

// parseJSONTagKey extracts the key name from a json tag.
func parseJSONTagKey(tag string) string {
	if tag == "" {
		return ""
	}
	if idx := strings.Index(tag, ","); idx >= 0 {
		return tag[:idx]
	}
	return tag
}

// deepCopyMap creates a deep copy of a map[string]any.
func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]any:
			result[k] = deepCopyMap(val)
		case []any:
			result[k] = deepCopySlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

// deepCopySlice creates a deep copy of a []any.
func deepCopySlice(s []any) []any {
	if s == nil {
		return nil
	}
	result := make([]any, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]any:
			result[i] = deepCopyMap(val)
		case []any:
			result[i] = deepCopySlice(val)
		default:
			result[i] = v
		}
	}
	return result
}
