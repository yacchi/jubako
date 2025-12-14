// Package document provides an abstraction for working with structured
// configuration documents while preserving comments and formatting.
//
// The Document interface is a pure format handler - it handles parsing and
// serialization only. Data loading and caching is managed by Source and Layer.
package document

// Document represents a format handler for configuration data.
//
// Document is stateless and does not cache data internally.
// It only knows how to:
//   - Parse bytes into map[string]any
//   - Apply patches to bytes and return new bytes
//   - Marshal test data
//
// Data source management (loading, caching, locking) is handled by Source and Layer.
type Document interface {
	// Get parses data bytes and returns content as map[string]any.
	// Returns empty map if data is nil or empty.
	//
	// Example:
	//   data, err := doc.Get(rawBytes)
	//   if err != nil {
	//     return err
	//   }
	//   port := data["server"].(map[string]any)["port"]
	Get(data []byte) (map[string]any, error)

	// Apply applies changeset to data bytes and returns new bytes.
	//
	// If the format supports AST-based updates (CanApply() returns true) and
	// changeset is provided: parses data, applies changeset operations
	// to preserve comments and formatting, then marshals the result.
	//
	// If changeset is empty or format doesn't support AST updates: parses
	// data and marshals it directly.
	//
	// The caller (typically Layer) handles actual file write via Source.Save.
	//
	// Example:
	//   newBytes, err := doc.Apply(currentBytes, changeset)
	//   if err != nil {
	//     return err
	//   }
	//   return source.Save(ctx, func(current []byte) ([]byte, error) {
	//     return doc.Apply(current, changeset)
	//   })
	Apply(data []byte, changeset JSONPatchSet) ([]byte, error)

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
	// that cannot be represented in this document format (e.g., null values
	// in TOML format).
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

