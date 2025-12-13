// Package json provides a standard library (encoding/json) implementation of
// the document.Document interface.
//
// This implementation is map-backed (map[string]any) and is intended for cases
// where comment/format preservation is not required.
package json

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/mapdoc"
)

// Document is a JSON document implementation backed by map[string]any.
type Document = mapdoc.Document

// New creates a new empty JSON document.
func New() *Document {
	return newDocument(make(map[string]any))
}

// Parse parses JSON data into a Document.
//
// The root value must be a JSON object. Empty/whitespace input is treated as an
// empty object.
func Parse(data []byte) (document.Document, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return New(), nil
	}

	var root any
	if err := json.Unmarshal(trimmed, &root); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if root == nil {
		return New(), nil
	}

	obj, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("failed to parse JSON: root must be an object, got %T", root)
	}

	return newDocument(obj), nil
}

func newDocument(data map[string]any) *Document {
	return mapdoc.New(
		document.FormatJSON,
		mapdoc.WithData(data),
		mapdoc.WithMarshal(marshalJSON),
	)
}

func marshalJSON(data map[string]any) ([]byte, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return append(b, '\n'), nil
}
