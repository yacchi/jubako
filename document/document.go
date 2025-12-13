// Package document provides an abstraction for working with structured
// configuration documents while preserving comments and formatting.
//
// The Document interface supports path-based access using JSON Pointer
// (RFC 6901) syntax, enabling precise navigation through nested structures.
package document

// Document represents a structured configuration document that can be
// read from and written to while preserving comments and formatting.
//
// Path syntax follows JSON Pointer (RFC 6901):
//   - "/server/port" - accesses server.port
//   - "/servers/0/name" - accesses servers[0].name
//   - "/feature~1flags/enabled" - accesses key "feature/flags" (escaped)
//
// See: https://tools.ietf.org/html/rfc6901
type Document interface {
	// Get retrieves the value at the specified JSON Pointer path.
	// Returns the value and true if found, or nil and false if not found.
	//
	// Example:
	//   value, ok := doc.Get("/server/port")
	//   if ok {
	//     port := value.(int)
	//   }
	Get(path string) (any, bool)

	// Set sets the value at the specified JSON Pointer path.
	// Creates intermediate nodes if they don't exist.
	// Returns an error if the path is invalid or the operation fails.
	//
	// Example:
	//   err := doc.Set("/server/port", 8080)
	Set(path string, value any) error

	// Delete removes the value at the specified JSON Pointer path.
	// Returns an error if the path is invalid or the operation fails.
	// Returns nil if the path doesn't exist (idempotent).
	//
	// Example:
	//   err := doc.Delete("/server/debug")
	Delete(path string) error

	// Marshal serializes the document to bytes, preserving comments
	// and formatting where possible.
	//
	// Example:
	//   data, err := doc.Marshal()
	//   if err != nil {
	//     return err
	//   }
	//   os.WriteFile("config.yaml", data, 0644)
	Marshal() ([]byte, error)

	// Format returns the document format type.
	//
	// Example:
	//   if doc.Format() == FormatYAML {
	//     // Handle YAML-specific logic
	//   }
	Format() DocumentFormat

	// MarshalTestData generates bytes that, when parsed, produce a document
	// containing the given data structure. This is intended for testing.
	//
	// Returns UnsupportedStructureError if the data contains structures
	// that cannot be represented in this document format (e.g., arrays
	// in environment variable format).
	//
	// Example:
	//   data := map[string]any{"server": map[string]any{"port": 8080}}
	//   bytes, err := doc.MarshalTestData(data)
	//   if errors.As(err, &document.UnsupportedStructureError{}) {
	//     // This structure is not supported by this format
	//   }
	MarshalTestData(data map[string]any) ([]byte, error)
}

// DocumentFormat represents the format of a configuration document.
type DocumentFormat string

const (
	// FormatYAML represents YAML format (using gopkg.in/yaml.v3).
	FormatYAML DocumentFormat = "yaml"

	// FormatTOML represents TOML format (using github.com/pelletier/go-toml/v2).
	FormatTOML DocumentFormat = "toml"

	// FormatJSONC represents JSON with Comments (using github.com/tailscale/hujson).
	FormatJSONC DocumentFormat = "jsonc"

	// FormatJSON represents standard JSON.
	FormatJSON DocumentFormat = "json"
)
