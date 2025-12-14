// Package jsonc provides a JSONC (JSON with comments) implementation of the
// document.Document interface.
//
// It preserves comments and formatting by operating on github.com/tailscale/hujson's AST.
// When no modifications are performed, Apply returns the input bytes verbatim.
package jsonc

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/tailscale/hujson"
	"github.com/yacchi/jubako/document"
)

// Document is a JSONC document implementation.
// It is stateless - parsing and serialization happen on demand.
type Document struct{}

// Ensure Document implements document.Document interface.
var _ document.Document = (*Document)(nil)

// New returns a JSONC Document.
//
// Example:
//
//	src := fs.New("~/.config/app.jsonc")
//	layer.New("user", src, jsonc.New())
func New() *Document {
	return &Document{}
}

// Format returns the document format.
func (d *Document) Format() document.DocumentFormat {
	return document.FormatJSONC
}

// Get parses data bytes and returns content as map[string]any.
// Returns empty map if data is nil or empty.
func (d *Document) Get(data []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return map[string]any{}, nil
	}

	v, err := hujson.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSONC: %w", err)
	}

	// Standardize to remove comments for decoding
	v.Standardize()

	var result map[string]any
	if err := json.Unmarshal(v.Pack(), &result); err != nil {
		return nil, fmt.Errorf("failed to decode JSONC: %w", err)
	}

	if result == nil {
		return map[string]any{}, nil
	}

	return result, nil
}

// Apply applies changeset to data bytes and returns new bytes.
// If changeset is provided: parses data, applies changeset operations
// using hujson's Patch API to preserve comments, then returns the result.
// If changeset is empty: marshals parsed data directly.
func (d *Document) Apply(data []byte, changeset document.JSONPatchSet) ([]byte, error) {
	// If no changeset, parse and re-marshal
	if changeset.IsEmpty() {
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) == 0 {
			return []byte("{}\n"), nil
		}

		v, err := hujson.Parse(trimmed)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JSONC: %w", err)
		}
		return v.Pack(), nil
	}

	// Parse existing data to preserve comments
	var root hujson.Value
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 {
		v, err := hujson.Parse(trimmed)
		if err != nil {
			// If parse fails, create new empty object
			v, _ = hujson.Parse([]byte("{}"))
			root = v
		} else {
			root = v
		}
	} else {
		// No existing data, create new empty object
		v, _ := hujson.Parse([]byte("{}"))
		root = v
	}

	// Apply each patch operation using hujson.Patch
	for _, patch := range changeset {
		var op string
		switch patch.Op {
		case document.PatchOpAdd:
			op = "add"
		case document.PatchOpReplace:
			op = "replace"
		case document.PatchOpRemove:
			op = "remove"
		default:
			continue
		}

		patchObj := map[string]any{
			"op":   op,
			"path": patch.Path,
		}
		if op == "add" || op == "replace" {
			patchObj["value"] = patch.Value
		}

		patchBytes, err := json.Marshal([]any{patchObj})
		if err != nil {
			continue
		}

		if err := root.Patch(patchBytes); err != nil {
			// If patch fails, continue with remaining patches
			continue
		}
	}

	return root.Pack(), nil
}

// MarshalTestData generates JSON bytes (without comments) for testing.
func (d *Document) MarshalTestData(data map[string]any) ([]byte, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSONC test data: %w", err)
	}
	return append(b, '\n'), nil
}
