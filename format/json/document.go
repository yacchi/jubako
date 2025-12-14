// Package json provides a standard library (encoding/json) implementation of
// the document.Document interface.
//
// This implementation does not preserve comments (standard JSON doesn't support them).
// Apply applies changeset operations in-memory and marshals the result.
package json

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/yacchi/jubako/document"
)

// Document is a JSON document implementation.
// It is stateless - parsing and serialization happen on demand.
type Document struct{}

// Ensure Document implements document.Document interface.
var _ document.Document = (*Document)(nil)

// New returns a JSON Document.
//
// Example:
//
//	src := fs.New("~/.config/app.json")
//	layer.New("user", src, json.New())
func New() *Document {
	return &Document{}
}

// Format returns the document format.
func (d *Document) Format() document.DocumentFormat {
	return document.FormatJSON
}

// Get parses data bytes and returns content as map[string]any.
// Returns empty map if data is nil or empty.
func (d *Document) Get(data []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return map[string]any{}, nil
	}

	var result map[string]any
	if err := json.Unmarshal(trimmed, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if result == nil {
		return map[string]any{}, nil
	}

	return result, nil
}

// Apply applies changeset to data bytes and returns new bytes.
// JSON does not support comments, so the changeset is applied in-memory
// and the result is marshaled directly.
func (d *Document) Apply(data []byte, changeset document.JSONPatchSet) ([]byte, error) {
	m, err := d.Get(data)
	if err != nil {
		return nil, err
	}

	changeset.ApplyTo(m)

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return append(b, '\n'), nil
}

// MarshalTestData generates JSON bytes for testing.
func (d *Document) MarshalTestData(data map[string]any) ([]byte, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON test data: %w", err)
	}
	return append(b, '\n'), nil
}
