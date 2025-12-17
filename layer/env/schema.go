// Package env provides an environment variable based configuration layer.
package env

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/yacchi/jubako/internal/tag"
	"github.com/yacchi/jubako/jsonptr"
)

// EnvMapping represents a single environment variable to field mapping.
type EnvMapping struct {
	// EnvVar is the environment variable name WITHOUT prefix (e.g., "SERVER_PORT").
	EnvVar string
	// JSONPath is the target JSON Pointer path (e.g., "/server/port").
	JSONPath string
	// FieldType is the target field type for automatic conversion.
	FieldType reflect.Type
}

// SchemaMapping holds all env var mappings derived from struct tags.
type SchemaMapping struct {
	// Mappings maps env var names (without prefix) to their mapping info.
	Mappings map[string]*EnvMapping
}

// BuildSchemaMapping analyzes struct T and extracts env: directives from jubako tags.
// It returns a SchemaMapping that can be used to create a TransformFunc.
//
// The env var names in tags should NOT include the prefix.
// For example, with prefix "APP_" and tag `jubako:"env:SERVER_PORT"`,
// the mapping will match environment variable "APP_SERVER_PORT".
//
// Example:
//
//	type Config struct {
//	    Port int    `json:"port" jubako:"env:SERVER_PORT"`
//	    Host string `json:"host" jubako:"env:SERVER_HOST"`
//	}
//
//	schema := BuildSchemaMapping[Config]()
func BuildSchemaMapping[T any]() *SchemaMapping {
	var zero T
	return buildSchemaMappingFromType(reflect.TypeOf(zero), "")
}

// buildSchemaMappingFromType recursively builds schema mapping from a struct type.
func buildSchemaMappingFromType(t reflect.Type, basePath string) *SchemaMapping {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return &SchemaMapping{Mappings: make(map[string]*EnvMapping)}
	}

	schema := &SchemaMapping{
		Mappings: make(map[string]*EnvMapping),
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Use shared tag parsing from internal/tag package
		tagInfo := tag.Parse(field, tag.DefaultFieldTagName, tag.DefaultDelimiter)

		// Skip if field key is "-" or skipped
		if tagInfo.FieldKey == "-" || tagInfo.Skipped {
			continue
		}

		// Build JSON path for this field
		fieldPath := basePath + "/" + jsonptr.Escape(tagInfo.FieldKey)

		// Check for env: directive
		if tagInfo.EnvVar != "" {
			// Use custom path if specified, otherwise use field path
			jsonPath := fieldPath
			if tagInfo.Path != "" {
				jsonPath = tagInfo.Path
			}

			schema.Mappings[tagInfo.EnvVar] = &EnvMapping{
				EnvVar:    tagInfo.EnvVar,
				JSONPath:  jsonPath,
				FieldType: unwrapPointer(field.Type),
			}
		}

		// Recursively process nested structs
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		if fieldType.Kind() == reflect.Struct && !isSpecialType(fieldType) {
			nested := buildSchemaMappingFromType(fieldType, fieldPath)
			for k, v := range nested.Mappings {
				if _, exists := schema.Mappings[k]; !exists {
					schema.Mappings[k] = v
				}
			}
		}
	}

	return schema
}

// unwrapPointer returns the underlying type if t is a pointer, otherwise returns t.
func unwrapPointer(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr {
		return t.Elem()
	}
	return t
}

// isSpecialType checks if a type should be treated as a leaf value (not recursed into).
func isSpecialType(t reflect.Type) bool {
	// time.Time should be serialized as a single value
	if t.PkgPath() == "time" && t.Name() == "Time" {
		return true
	}
	return false
}

// CreateTransformFunc generates a TransformFunc from the schema mapping.
// The returned TransformFunc will:
// 1. Look up the env var name (without prefix) in the schema
// 2. Convert the string value to the target field type
// 3. Return the JSON Pointer path for that field
//
// Unmapped env vars or type conversion failures will return an empty path,
// causing the env layer to skip that variable.
func (s *SchemaMapping) CreateTransformFunc() TransformFunc {
	return func(key, value string) (path string, finalValue any) {
		mapping, ok := s.Mappings[key]
		if !ok {
			return "", nil // Skip unmapped vars
		}

		converted, err := convertStringToType(value, mapping.FieldType)
		if err != nil {
			return "", nil // Skip on conversion error
		}

		return mapping.JSONPath, converted
	}
}

// convertStringToType converts a string value to the target type.
// Supported types: string, int*, uint*, float*, bool, []string, time.Duration.
func convertStringToType(value string, targetType reflect.Type) (any, error) {
	// Handle pointer types
	if targetType.Kind() == reflect.Ptr {
		targetType = targetType.Elem()
	}

	switch targetType.Kind() {
	case reflect.String:
		return value, nil

	case reflect.Int:
		v, err := strconv.ParseInt(value, 10, 0)
		return int(v), err
	case reflect.Int8:
		v, err := strconv.ParseInt(value, 10, 8)
		return int8(v), err
	case reflect.Int16:
		v, err := strconv.ParseInt(value, 10, 16)
		return int16(v), err
	case reflect.Int32:
		v, err := strconv.ParseInt(value, 10, 32)
		return int32(v), err
	case reflect.Int64:
		// Check for time.Duration
		if targetType.PkgPath() == "time" && targetType.Name() == "Duration" {
			return time.ParseDuration(value)
		}
		v, err := strconv.ParseInt(value, 10, 64)
		return v, err

	case reflect.Uint:
		v, err := strconv.ParseUint(value, 10, 0)
		return uint(v), err
	case reflect.Uint8:
		v, err := strconv.ParseUint(value, 10, 8)
		return uint8(v), err
	case reflect.Uint16:
		v, err := strconv.ParseUint(value, 10, 16)
		return uint16(v), err
	case reflect.Uint32:
		v, err := strconv.ParseUint(value, 10, 32)
		return uint32(v), err
	case reflect.Uint64:
		v, err := strconv.ParseUint(value, 10, 64)
		return v, err

	case reflect.Float32:
		v, err := strconv.ParseFloat(value, 32)
		return float32(v), err
	case reflect.Float64:
		v, err := strconv.ParseFloat(value, 64)
		return v, err

	case reflect.Bool:
		return strconv.ParseBool(value)

	case reflect.Slice:
		// Support comma-separated string slices
		if targetType.Elem().Kind() == reflect.String {
			if value == "" {
				return []string{}, nil
			}
			return strings.Split(value, ","), nil
		}
		// For other slice types, return as string
		return value, nil

	default:
		// Unknown type, return as string
		return value, nil
	}
}

// WithSchemaMapping creates a TransformFunc from struct tag mappings.
// Environment variable names in tags should NOT include the prefix.
//
// Example:
//
//	type Config struct {
//	    Port int    `json:"port" jubako:"env:SERVER_PORT"`
//	    Host string `json:"host" jubako:"env:SERVER_HOST"`
//	}
//
//	env.New("env", "APP_", env.WithSchemaMapping[Config]())
//	// APP_SERVER_PORT=8080 -> /port = 8080
//	// APP_SERVER_HOST=localhost -> /host = "localhost"
func WithSchemaMapping[T any]() Option {
	schema := BuildSchemaMapping[T]()
	return func(l *Layer) {
		l.transform = schema.CreateTransformFunc()
	}
}
