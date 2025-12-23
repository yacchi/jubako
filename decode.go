package jubako

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/yacchi/jubako/container"
	"github.com/yacchi/jubako/decoder"
	"github.com/yacchi/jubako/internal/tag"
	"github.com/yacchi/jubako/jsonptr"
)

// DefaultTagDelimiter is the default delimiter used to separate path and directives
// in jubako struct tags. This can be changed via WithTagDelimiter option.
const DefaultTagDelimiter = tag.DefaultDelimiter

// DefaultFieldTagName is the default struct tag name used for field name resolution.
// This follows the same convention as encoding/json.
const DefaultFieldTagName = tag.DefaultFieldTagName

// sensitiveState represents the sensitivity state of a field.
// This is an alias for the internal tag.SensitiveState type.
type sensitiveState = tag.SensitiveState

// Sensitivity state constants.
const (
	sensitiveNone     = tag.SensitiveNone
	sensitiveExplicit = tag.SensitiveExplicit
)

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
	// Sensitive indicates whether this field contains sensitive data.
	// Used to prevent cross-contamination between sensitive and non-sensitive layers.
	Sensitive sensitiveState
	// FieldType is the reflect.Type of the struct field.
	// Used for type conversion when setting values via SetTo.
	FieldType reflect.Type
}

// HasDirective returns true if this mapping has any jubako tag directives.
// This is used to distinguish between fields that only have type info
// (for SetTo conversion) vs fields with actual jubako tag mappings.
func (m *PathMapping) HasDirective() bool {
	return m.SourcePath != "" || m.Skipped || m.Sensitive == sensitiveExplicit
}

// MappingTable holds all path mappings for a struct type.
// Built once at Store initialization, used during every materialize.
//
// This is a simplified design that removes redundant lookup maps.
// Sensitivity checking is delegated to SensitiveTrie which is built
// from this table at Store initialization.
type MappingTable struct {
	// Mappings contains direct field mappings at this level.
	Mappings []*PathMapping
	// Nested contains mapping tables for nested struct fields (key = JSON key).
	Nested map[string]*MappingTable
	// SliceElement contains mapping table for slice element type (if slice of structs).
	SliceElement map[string]*MappingTable
	// MapValue contains mapping table for map value type (if map with struct values).
	MapValue map[string]*MappingTable
}

// buildMappingTable creates a mapping table for the given struct type.
// This is called once during Store initialization.
// The delimiter is used to separate path and directives in jubako struct tags.
// The fieldTagName specifies which struct tag to use for field name resolution (e.g., "json", "yaml").
func buildMappingTable(t reflect.Type, delimiter string, fieldTagName string) *MappingTable {
	return buildMappingTableRecursive(t, delimiter, fieldTagName, "")
}

// buildMappingTableRecursive creates a mapping table with type path tracking for warnings.
func buildMappingTableRecursive(t reflect.Type, delimiter string, fieldTagName string, typePath string) *MappingTable {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	// Build type path for warnings
	currentTypePath := t.Name()
	if typePath != "" {
		currentTypePath = typePath
	}

	table := &MappingTable{
		Mappings:     make([]*PathMapping, 0),
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

		// Parse all struct tags at once
		tagInfo := tag.Parse(field, fieldTagName, delimiter)

		// Skip if field key is "-" (json:"-")
		if tagInfo.FieldKey == "-" {
			continue
		}

		// Check for sensitive tag on non-leaf types and emit warning
		checkSensitiveOnNonLeaf(currentTypePath, field, tagInfo)

		// Create PathMapping for all fields to support type conversion in SetTo.
		// Even fields without jubako tags need type info for proper conversion.
		m := &PathMapping{
			FieldKey:   tagInfo.FieldKey,
			SourcePath: tagInfo.Path,
			IsRelative: tagInfo.IsRelative,
			Sensitive:  tagInfo.Sensitive,
			Skipped:    tagInfo.Skipped,
			FieldType:  field.Type,
		}
		table.Mappings = append(table.Mappings, m)

		// Check for nested types (struct, slice, map) that may have jubako tags
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		// Build nested type path for warnings
		nestedTypePath := currentTypePath + "." + field.Name

		switch fieldType.Kind() {
		case reflect.Struct:
			// Recursively build mapping table for nested struct
			if nested := buildMappingTableRecursive(fieldType, delimiter, fieldTagName, nestedTypePath); nested != nil && !nested.IsEmpty() {
				table.Nested[tagInfo.FieldKey] = nested
			}

		case reflect.Slice, reflect.Array:
			// Check if slice element is a struct with mappings
			elemType := fieldType.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct {
				elemTypePath := nestedTypePath + "[]"
				if elemTable := buildMappingTableRecursive(elemType, delimiter, fieldTagName, elemTypePath); elemTable != nil && !elemTable.IsEmpty() {
					table.SliceElement[tagInfo.FieldKey] = elemTable
				}
			}

		case reflect.Map:
			// Check if map value is a struct with mappings
			valueType := fieldType.Elem()
			if valueType.Kind() == reflect.Ptr {
				valueType = valueType.Elem()
			}
			if valueType.Kind() == reflect.Struct {
				mapTypePath := nestedTypePath + "[key]"
				if valueTable := buildMappingTableRecursive(valueType, delimiter, fieldTagName, mapTypePath); valueTable != nil && !valueTable.IsEmpty() {
					table.MapValue[tagInfo.FieldKey] = valueTable
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

// IsEmpty returns true if there are no jubako tag directives defined.
// Note: Mappings may contain entries for type conversion purposes,
// but IsEmpty only considers entries with actual jubako tag directives.
func (t *MappingTable) IsEmpty() bool {
	if t == nil {
		return true
	}
	// Check if any mapping has a directive
	for _, m := range t.Mappings {
		if m.HasDirective() {
			return false
		}
	}
	// Check nested tables
	if len(t.Nested) > 0 || len(t.SliceElement) > 0 || len(t.MapValue) > 0 {
		return false
	}
	return true
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
		dst = container.DeepCopyMap(src)
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
			if value, ok := jsonptr.GetPath(lookupSrc, m.SourcePath); ok {
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

