// Package env provides an environment variable based configuration layer.
// This layer is read-only and CanSave() returns false.
package env

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/mapdoc"
)

// Layer loads configuration from environment variables with a specified prefix.
// Environment variable names are converted to configuration paths by:
// 1. Removing the prefix
// 2. Converting to lowercase
// 3. Replacing underscores with nested paths
//
// Example: with prefix "APP_", environment variable "APP_SERVER_PORT=8080"
// becomes path "/server/port" with value "8080".
//
// This layer is read-only and does not support saving.
type Layer struct {
	name   layer.Name
	prefix string
	doc    *envDocument
}

// Ensure Layer implements layer.Layer interface.
var _ layer.Layer = (*Layer)(nil)

// New creates a new environment variable layer.
//
// The prefix is used to filter environment variables. Only variables starting
// with the prefix are included. The prefix is stripped from variable names
// when creating configuration paths.
//
// Example:
//
//	// Load all environment variables starting with "APP_"
//	envLayer := env.New("env", "APP_")
//
//	// With APP_SERVER_PORT=8080 and APP_DATABASE_HOST=localhost:
//	// - /server/port = "8080"
//	// - /database/host = "localhost"
func New(name layer.Name, prefix string) *Layer {
	return &Layer{
		name:   name,
		prefix: prefix,
	}
}

// Name returns the layer's name.
func (l *Layer) Name() layer.Name {
	return l.name
}

// Load reads environment variables and creates a Document.
func (l *Layer) Load(ctx context.Context) (document.Document, error) {
	// Check for cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data := make(map[string]any)

	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}

		key, value := pair[0], pair[1]

		// Check if the key has our prefix
		if !strings.HasPrefix(key, l.prefix) {
			continue
		}

		// Remove prefix and convert to path
		key = strings.TrimPrefix(key, l.prefix)
		path := envKeyToPath(key)

		// Set the value in the nested map
		mapdoc.Set(data, path, value)
	}

	l.doc = &envDocument{data: data}
	return l.doc, nil
}

// Document returns the last loaded document.
func (l *Layer) Document() document.Document {
	if l.doc == nil {
		return nil
	}
	return l.doc
}

// Prefix returns the environment variable prefix.
func (l *Layer) Prefix() string {
	return l.prefix
}

// Save is not supported for environment variable layers.
// Environment variables are inherently read-only in this implementation.
func (l *Layer) Save(ctx context.Context) error {
	return fmt.Errorf("environment variable layer %q does not support saving", l.name)
}

// CanSave returns false because environment variable layers are read-only.
func (l *Layer) CanSave() bool {
	return false
}

// envKeyToPath converts an environment variable key to a path.
// Example: "SERVER_PORT" -> []string{"server", "port"}
func envKeyToPath(key string) []string {
	key = strings.ToLower(key)
	return strings.Split(key, "_")
}

// envDocument is a simple Document implementation for environment variables.
type envDocument struct {
	data map[string]any
}

// Ensure envDocument implements document.Document interface.
var _ document.Document = (*envDocument)(nil)

// Get retrieves a value at the given JSON Pointer path.
func (d *envDocument) Get(path string) (any, bool) {
	if path == "" || path == "/" {
		return d.data, true
	}

	// Parse JSON Pointer path
	parts := parseJSONPointer(path)
	return mapdoc.Get(d.data, parts)
}

// Set is not supported for environment variable documents.
// Environment variables are read-only in this implementation.
func (d *envDocument) Set(path string, value any) error {
	return document.Unsupported("environment variables are read-only")
}

// Delete is not supported for environment variable documents.
// Environment variables are read-only in this implementation.
func (d *envDocument) Delete(path string) error {
	return document.Unsupported("environment variables are read-only")
}

// Marshal is not meaningfully supported for environment variables.
func (d *envDocument) Marshal() ([]byte, error) {
	// This shouldn't be called since Layer doesn't implement WritableLayer
	return nil, nil
}

// Format returns the document format.
func (d *envDocument) Format() document.DocumentFormat {
	return "env"
}

// parseJSONPointer parses a JSON Pointer path into parts using RFC 6901 escaping.
func parseJSONPointer(path string) []string {
	keys, err := jsonptr.Parse(path)
	if err != nil {
		// Fallback for invalid paths: treat as simple split
		if path == "" || path == "/" {
			return nil
		}
		if strings.HasPrefix(path, "/") {
			path = path[1:]
		}
		return strings.Split(path, "/")
	}
	return keys
}

// MarshalTestData generates test data for environment variable format.
// Environment variables have limitations:
//   - Arrays are not supported
//   - All values are strings
//   - Nested maps are flattened with underscore separators
//
// Returns UnsupportedStructureError for arrays or non-string leaf values
// that cannot be represented as environment variables.
func (d *envDocument) MarshalTestData(data map[string]any) ([]byte, error) {
	var lines []string
	if err := marshalEnvData("", data, &lines); err != nil {
		return nil, err
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// marshalEnvData recursively converts map data to KEY=value lines.
func marshalEnvData(prefix string, data map[string]any, lines *[]string) error {
	for key, value := range data {
		envKey := strings.ToUpper(key)
		if prefix != "" {
			envKey = prefix + "_" + envKey
		}

		switch v := value.(type) {
		case map[string]any:
			if err := marshalEnvData(envKey, v, lines); err != nil {
				return err
			}
		case []any:
			path := "/" + strings.ToLower(strings.ReplaceAll(envKey, "_", "/"))
			return document.UnsupportedAt(path, "arrays not supported in environment variables")
		case string:
			*lines = append(*lines, envKey+"="+v)
		case nil:
			// Skip nil values - env vars cannot represent null
			path := "/" + strings.ToLower(strings.ReplaceAll(envKey, "_", "/"))
			return document.UnsupportedAt(path, "null values not supported in environment variables")
		default:
			// Convert other types to string representation
			*lines = append(*lines, fmt.Sprintf("%s=%v", envKey, v))
		}
	}
	return nil
}
