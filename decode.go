package jubako

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/yacchi/jubako/container"
	"github.com/yacchi/jubako/decoder"
	"github.com/yacchi/jubako/jsonptr"
)

const tagName = "jubako"

// DefaultTagDelimiter is the default delimiter used to separate path and directives
// in jubako struct tags. This can be changed via WithTagDelimiter option.
const DefaultTagDelimiter = ","

// DefaultFieldTagName is the default struct tag name used for field name resolution.
// This follows the same convention as encoding/json.
const DefaultFieldTagName = "json"

// sensitiveState represents the sensitivity state of a field.
type sensitiveState int

const (
	// sensitiveInherit means the field inherits sensitivity from its parent.
	sensitiveInherit sensitiveState = iota
	// sensitiveExplicit means the field is explicitly marked as sensitive.
	sensitiveExplicit
	// sensitiveExplicitNot means the field is explicitly marked as NOT sensitive,
	// overriding any inherited sensitivity.
	sensitiveExplicitNot
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
}

// MappingTable holds all path mappings for a struct type.
// Built once at Store initialization, used during every materialize.
type MappingTable struct {
	// Mappings contains direct field mappings at this level (for iteration).
	// Use MappingByKey for O(1) lookup by field key.
	Mappings []*PathMapping
	// MappingByKey provides O(1) lookup of PathMapping by JSON field key.
	// Points to the same PathMapping instances as Mappings slice.
	MappingByKey map[string]*PathMapping
	// Nested contains mapping tables for nested struct fields (key = JSON key).
	Nested map[string]*MappingTable
	// SliceElement contains mapping table for slice element type (if slice of structs).
	SliceElement map[string]*MappingTable
	// MapValue contains mapping table for map value type (if map with struct values).
	MapValue map[string]*MappingTable

	// sensitive is the sensitivity state of this table's struct itself.
	// Used for inheriting sensitivity to nested fields.
	sensitive sensitiveState
}

// buildMappingTable creates a mapping table for the given struct type.
// This is called once during Store initialization.
// The delimiter is used to separate path and directives in jubako struct tags.
// The fieldTagName specifies which struct tag to use for field name resolution (e.g., "json", "yaml").
func buildMappingTable(t reflect.Type, delimiter string, fieldTagName string) *MappingTable {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	table := &MappingTable{
		Mappings:     make([]*PathMapping, 0),
		MappingByKey: make(map[string]*PathMapping),
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

		fieldKey := getFieldKey(field, fieldTagName)
		if fieldKey == "-" {
			continue
		}

		// Check for jubako tag
		tag, hasTag := field.Tag.Lookup(tagName)
		if hasTag {
			if tag == "-" {
				m := &PathMapping{
					FieldKey: fieldKey,
					Skipped:  true,
				}
				table.Mappings = append(table.Mappings, m)
				table.MappingByKey[fieldKey] = m
			} else {
				path, isRelative, sensitive := parseJubakoTag(tag, delimiter)
				if path != "" || sensitive != sensitiveInherit {
					m := &PathMapping{
						FieldKey:   fieldKey,
						SourcePath: path,
						IsRelative: isRelative,
						Sensitive:  sensitive,
					}
					table.Mappings = append(table.Mappings, m)
					table.MappingByKey[fieldKey] = m
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
			if nested := buildMappingTable(fieldType, delimiter, fieldTagName); nested != nil && !nested.IsEmpty() {
				table.Nested[fieldKey] = nested
			}

		case reflect.Slice, reflect.Array:
			// Check if slice element is a struct with mappings
			elemType := fieldType.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct {
				if elemTable := buildMappingTable(elemType, delimiter, fieldTagName); elemTable != nil && !elemTable.IsEmpty() {
					table.SliceElement[fieldKey] = elemTable
				}
			}

		case reflect.Map:
			// Check if map value is a struct with mappings
			valueType := fieldType.Elem()
			if valueType.Kind() == reflect.Ptr {
				valueType = valueType.Elem()
			}
			if valueType.Kind() == reflect.Struct {
				if valueTable := buildMappingTable(valueType, delimiter, fieldTagName); valueTable != nil && !valueTable.IsEmpty() {
					table.MapValue[fieldKey] = valueTable
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

// IsSensitive checks if the given JSONPointer path is sensitive.
// It traverses the mapping table hierarchy and handles sensitivity inheritance.
func (t *MappingTable) IsSensitive(path string) bool {
	if t == nil {
		return false
	}

	// Parse the path into segments
	segments, err := jsonptr.Parse(path)
	if err != nil || len(segments) == 0 {
		return false
	}

	return t.isSensitiveRecursive(segments, false)
}

// isSensitiveRecursive traverses the mapping table to determine sensitivity.
// parentSensitive indicates if an ancestor was marked sensitive.
func (t *MappingTable) isSensitiveRecursive(segments []string, parentSensitive bool) bool {
	if t == nil || len(segments) == 0 {
		return parentSensitive
	}

	key := segments[0]
	remaining := segments[1:]

	// Check if this table itself is marked sensitive (for nested structs)
	currentSensitive := parentSensitive
	if t.sensitive == sensitiveExplicit {
		currentSensitive = true
	} else if t.sensitive == sensitiveExplicitNot {
		currentSensitive = false
	}

	// Look for this key in MappingByKey (O(1) lookup) to check field-level sensitivity
	if m, ok := t.MappingByKey[key]; ok {
		// Determine sensitivity for this specific field
		switch m.Sensitive {
		case sensitiveExplicit:
			currentSensitive = true
		case sensitiveExplicitNot:
			currentSensitive = false
			// sensitiveInherit: keep currentSensitive
		}
	}

	// If no more segments, return current sensitivity
	if len(remaining) == 0 {
		return currentSensitive
	}

	// Look for nested table
	if nested, ok := t.Nested[key]; ok {
		return nested.isSensitiveRecursive(remaining, currentSensitive)
	}

	// Check slice element (for paths like /items/0/field)
	if elemTable, ok := t.SliceElement[key]; ok {
		// Skip the index segment (e.g., "0") and continue with the rest
		if len(remaining) > 1 {
			return elemTable.isSensitiveRecursive(remaining[1:], currentSensitive)
		}
		return currentSensitive
	}

	// Check map value (for paths like /settings/key/field)
	if valueTable, ok := t.MapValue[key]; ok {
		// Skip the key segment and continue with the rest
		if len(remaining) > 1 {
			return valueTable.isSensitiveRecursive(remaining[1:], currentSensitive)
		}
		return currentSensitive
	}

	// No nested table found, return inherited sensitivity
	return currentSensitive
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

// getFieldKey returns the field key for a struct field based on the specified tag name.
// This follows the same convention as encoding/json:
// - If the specified tag exists and has a key, that key is used
// - Otherwise, the struct field name is used as-is
//
// The tag value is split by comma and the first segment is used as the key,
// matching the behavior of encoding/json and similar libraries.
func getFieldKey(field reflect.StructField, tagName string) string {
	tag := field.Tag.Get(tagName)
	if tag == "" {
		return field.Name
	}
	key := parseJSONTagKey(tag)
	if key == "" {
		return field.Name
	}
	return key
}

// parseJubakoTag extracts the path, relative flag, and sensitivity state from a jubako tag.
//
// Path formats:
//   - "/path/to/value" - absolute path from root
//   - "path/to/value"  - relative path from current context
//   - "./path/to/value" - relative path (explicit, "./" is stripped)
//
// Directives (delimiter-separated, default ","):
//   - "sensitive" - marks field as containing sensitive data
//   - "!sensitive" - explicitly marks field as NOT sensitive (overrides inheritance)
//
// Examples (with default delimiter ","):
//   - `jubako:"sensitive"` - sensitive field, no path remap
//   - `jubako:"/path,sensitive"` - sensitive field with absolute path remap
//   - `jubako:"!sensitive"` - explicitly non-sensitive (opts out of inherited sensitivity)
//
// For config files with commas in key names, use WithTagDelimiter to change the delimiter:
//   - With delimiter ";": `jubako:"/path,with,commas;sensitive"`
func parseJubakoTag(tag string, delimiter string) (path string, isRelative bool, sensitive sensitiveState) {
	sensitive = sensitiveInherit

	// Split by delimiter to get path and directives
	parts := strings.Split(tag, delimiter)
	pathPart := ""
	if len(parts) > 0 {
		pathPart = strings.TrimSpace(parts[0])
	}

	// Parse directives
	for i := 1; i < len(parts); i++ {
		directive := strings.TrimSpace(parts[i])
		switch directive {
		case "sensitive":
			sensitive = sensitiveExplicit
		case "!sensitive":
			sensitive = sensitiveExplicitNot
		}
	}

	// Check if first part is just a directive (no path)
	switch pathPart {
	case "sensitive":
		return "", false, sensitiveExplicit
	case "!sensitive":
		return "", false, sensitiveExplicitNot
	}

	// Check for explicit relative prefix
	if strings.HasPrefix(pathPart, "./") {
		return "/" + pathPart[2:], true, sensitive // Convert "./foo" to "/foo" for JSONPointer
	}

	// Absolute paths start with "/"
	if strings.HasPrefix(pathPart, "/") {
		return pathPart, false, sensitive
	}

	// No leading "/" means relative path
	if pathPart != "" {
		return "/" + pathPart, true, sensitive // Prepend "/" for JSONPointer format
	}

	return pathPart, false, sensitive
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
