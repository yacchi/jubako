// Package tag provides internal struct tag parsing utilities for jubako.
package tag

import (
	"reflect"
	"strings"
)

const (
	// JubakoTagName is the struct tag name used for jubako directives.
	JubakoTagName = "jubako"

	// DefaultDelimiter is the default delimiter used to separate path and directives
	// in jubako struct tags.
	DefaultDelimiter = ","

	// DefaultFieldTagName is the default struct tag name used for field name resolution.
	DefaultFieldTagName = "json"
)

// SensitiveState represents the sensitivity state of a field.
type SensitiveState int

const (
	// SensitiveNone means the field is not sensitive (default).
	SensitiveNone SensitiveState = iota
	// SensitiveExplicit means the field is explicitly marked as sensitive.
	// Use `jubako:"sensitive"` or `jubako:"sensitive=true"` to mark a field.
	SensitiveExplicit
)

// FieldInfo contains all parsed tag information for a struct field.
// This struct consolidates json/yaml tag and jubako tag parsing results.
// It is designed to be extensible for future directives.
type FieldInfo struct {
	// FieldKey is the JSON/YAML key for this field (from fieldTagName tag or field name).
	// "-" means the field should be skipped entirely.
	FieldKey string

	// FieldType is the reflect.Type of the field (with pointers unwrapped).
	FieldType reflect.Type

	// Skipped is true if the field has jubako:"-" tag.
	Skipped bool

	// Path is the JSONPointer path (e.g., "/server/port") from jubako tag.
	// Empty if no path is specified.
	Path string

	// IsRelative is true if the path is relative to current context.
	IsRelative bool

	// Sensitive indicates the sensitivity state of this field.
	Sensitive SensitiveState

	// EnvVar is the environment variable name (without prefix) from env: directive.
	// Empty if no env: directive is present.
	EnvVar string
}

// Parse parses all relevant struct tags for a field and returns FieldInfo.
// This function consolidates json/yaml tag and jubako tag parsing.
//
// Parameters:
//   - field: the struct field to parse tags from
//   - fieldTagName: the tag name for field key (e.g., "json", "yaml")
//   - jubakoDelimiter: delimiter for jubako tag directives (default ",")
//
// The fieldTagName tag is used to determine the field key for serialization.
// The jubako tag is used for path remapping, sensitivity, and env var mapping.
func Parse(field reflect.StructField, fieldTagName string, jubakoDelimiter string) FieldInfo {
	info := FieldInfo{
		Sensitive: SensitiveNone,
		FieldType: unwrapPointer(field.Type),
	}

	// Parse field key from fieldTagName tag (e.g., json, yaml)
	info.FieldKey = ParseFieldKey(field, fieldTagName)

	// If field key is "-", the field should be skipped
	if info.FieldKey == "-" {
		return info
	}

	// Parse jubako tag if present
	jubakoTag, hasJubakoTag := field.Tag.Lookup(JubakoTagName)
	if !hasJubakoTag {
		return info
	}

	// Check for skip directive
	if jubakoTag == "-" {
		info.Skipped = true
		return info
	}

	// Parse jubako tag directives
	ParseJubakoDirectives(jubakoTag, jubakoDelimiter, &info)

	return info
}

// unwrapPointer returns the underlying type if t is a pointer, otherwise returns t.
func unwrapPointer(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr {
		return t.Elem()
	}
	return t
}

// ParseFieldKey extracts the field key from a struct tag.
// This follows the same convention as encoding/json:
// - If the specified tag exists and has a key, that key is used
// - Otherwise, the struct field name is used as-is
func ParseFieldKey(field reflect.StructField, tagName string) string {
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

// ParseJubakoDirectives parses a jubako tag and populates the FieldInfo.
//
// Path formats:
//   - "/path/to/value" - absolute path from root
//   - "path/to/value"  - relative path from current context
//   - "./path/to/value" - relative path (explicit, "./" is stripped)
//
// Directives (delimiter-separated, default ","):
//   - "sensitive" or "sensitive=true" - marks field as containing sensitive data
//   - "env:VAR_NAME" - maps environment variable VAR_NAME to this field
//
// Examples (with default delimiter ","):
//   - `jubako:"sensitive"` - sensitive field, no path remap
//   - `jubako:"sensitive=true"` - same as above (explicit form)
//   - `jubako:"/path,sensitive"` - sensitive field with absolute path remap
//   - `jubako:"env:SERVER_PORT"` - map env var SERVER_PORT to this field
//   - `jubako:"/path,env:PORT,sensitive"` - path remap + env mapping + sensitive
func ParseJubakoDirectives(tag string, delimiter string, info *FieldInfo) {
	// Split by delimiter to get path and directives
	parts := strings.Split(tag, delimiter)
	pathPart := ""
	if len(parts) > 0 {
		pathPart = strings.TrimSpace(parts[0])
	}

	// Parse directives
	for i := 1; i < len(parts); i++ {
		directive := strings.TrimSpace(parts[i])
		switch {
		case directive == "sensitive", directive == "sensitive=true":
			info.Sensitive = SensitiveExplicit
		case strings.HasPrefix(directive, "env:"):
			info.EnvVar = strings.TrimPrefix(directive, "env:")
		}
	}

	// Check if first part is just a directive (no path)
	switch {
	case pathPart == "sensitive", pathPart == "sensitive=true":
		info.Sensitive = SensitiveExplicit
		return
	case strings.HasPrefix(pathPart, "env:"):
		// First part is env directive (no path)
		info.EnvVar = strings.TrimPrefix(pathPart, "env:")
		return
	}

	// Check for explicit relative prefix
	if strings.HasPrefix(pathPart, "./") {
		info.Path = "/" + pathPart[2:] // Convert "./foo" to "/foo" for JSONPointer
		info.IsRelative = true
		return
	}

	// Absolute paths start with "/"
	if strings.HasPrefix(pathPart, "/") {
		info.Path = pathPart
		return
	}

	// No leading "/" means relative path
	if pathPart != "" {
		info.Path = "/" + pathPart // Prepend "/" for JSONPointer format
		info.IsRelative = true
	}
}

// ParseJubakoTag is a convenience function for parsing just a jubako tag string.
// Used for testing and cases where only the jubako tag needs to be parsed.
func ParseJubakoTag(tag string, delimiter string) FieldInfo {
	info := FieldInfo{
		Sensitive: SensitiveNone,
	}
	if tag == "-" {
		info.Skipped = true
		return info
	}
	ParseJubakoDirectives(tag, delimiter, &info)
	return info
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
