package main

import (
	"context"
	"strings"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
)

// PathMapping defines a bidirectional path transformation.
type PathMapping struct {
	// Canonical is the path used by the application (v2 format)
	Canonical string
	// Source is the path in the underlying document (v1 format)
	Source string
}

// TransformLayer wraps a Layer and transforms paths bidirectionally.
// This enables reading/writing with a canonical schema while the
// underlying document uses a different structure.
type TransformLayer struct {
	inner    layer.Layer
	mappings []PathMapping
	doc      *TransformedDocument
}

// NewTransformLayer creates a new TransformLayer that wraps the given layer.
func NewTransformLayer(inner layer.Layer, mappings []PathMapping) *TransformLayer {
	return &TransformLayer{
		inner:    inner,
		mappings: mappings,
	}
}

func (t *TransformLayer) Name() layer.Name {
	return t.inner.Name()
}

func (t *TransformLayer) Load(ctx context.Context) (document.Document, error) {
	doc, err := t.inner.Load(ctx)
	if err != nil {
		return nil, err
	}
	t.doc = NewTransformedDocument(doc, t.mappings)
	return t.doc, nil
}

func (t *TransformLayer) Document() document.Document {
	if t.doc == nil {
		innerDoc := t.inner.Document()
		if innerDoc == nil {
			return nil
		}
		t.doc = NewTransformedDocument(innerDoc, t.mappings)
	}
	return t.doc
}

func (t *TransformLayer) Save(ctx context.Context) error {
	return t.inner.Save(ctx)
}

func (t *TransformLayer) CanSave() bool {
	return t.inner.CanSave()
}

// TransformedDocument wraps a Document and transforms paths bidirectionally.
// When Get("") is called (root access), it transforms the entire data structure
// using the path mappings. This enables seamless integration with Store.
type TransformedDocument struct {
	inner    document.Document
	mappings []PathMapping
	// Pre-built lookup maps for efficient transformation
	canonicalToSource map[string]string
	sourceToCanonical map[string]string
}

// NewTransformedDocument creates a TransformedDocument with the given mappings.
func NewTransformedDocument(inner document.Document, mappings []PathMapping) *TransformedDocument {
	d := &TransformedDocument{
		inner:             inner,
		mappings:          mappings,
		canonicalToSource: make(map[string]string),
		sourceToCanonical: make(map[string]string),
	}
	for _, m := range mappings {
		d.canonicalToSource[m.Canonical] = m.Source
		d.sourceToCanonical[m.Source] = m.Canonical
	}
	return d
}

// toSourcePath converts a canonical path to the source path.
func (d *TransformedDocument) toSourcePath(canonicalPath string) string {
	if sourcePath, ok := d.canonicalToSource[canonicalPath]; ok {
		return sourcePath
	}
	return canonicalPath // No mapping, use as-is
}

// toCanonicalPath converts a source path to the canonical path.
func (d *TransformedDocument) toCanonicalPath(sourcePath string) string {
	if canonicalPath, ok := d.sourceToCanonical[sourcePath]; ok {
		return canonicalPath
	}
	return sourcePath // No mapping, use as-is
}

// Get retrieves the value at the canonical path by transforming to source path.
// Special case: when path is "" (root), transforms the entire data structure.
func (d *TransformedDocument) Get(path string) (any, bool) {
	if path == "" {
		// Root access - transform entire structure
		return d.getTransformedRoot()
	}
	sourcePath := d.toSourcePath(path)
	return d.inner.Get(sourcePath)
}

// getTransformedRoot builds a new map with canonical paths from source data.
func (d *TransformedDocument) getTransformedRoot() (any, bool) {
	rootValue, ok := d.inner.Get("")
	if !ok {
		return nil, false
	}

	// Build canonical structure by fetching each mapped value
	result := make(map[string]any)

	for canonical, source := range d.canonicalToSource {
		value, exists := d.inner.Get(source)
		if !exists {
			continue
		}
		setValueAtPath(result, canonical, value)
	}

	// Also include unmapped values from root (passthrough)
	if rootMap, ok := rootValue.(map[string]any); ok {
		d.copyUnmappedValues(result, rootMap, "")
	}

	return result, true
}

// copyUnmappedValues copies values that don't have explicit mappings.
func (d *TransformedDocument) copyUnmappedValues(dst map[string]any, src map[string]any, prefix string) {
	for key, value := range src {
		srcPath := prefix + "/" + jsonptr.Escape(key)

		// Check if this path or any child has a mapping
		if d.hasMapping(srcPath) {
			// If it's a map, recurse to check children
			if nested, ok := value.(map[string]any); ok {
				d.copyUnmappedValues(dst, nested, srcPath)
			}
			continue
		}

		// No mapping for this path - copy as-is to canonical structure
		canonicalPath := d.toCanonicalPath(srcPath)
		setValueAtPath(dst, canonicalPath, value)
	}
}

// hasMapping checks if a source path has any mapping (exact or prefix match).
func (d *TransformedDocument) hasMapping(sourcePath string) bool {
	for _, src := range d.canonicalToSource {
		if src == sourcePath || strings.HasPrefix(src, sourcePath+"/") {
			return true
		}
	}
	return false
}

// setValueAtPath sets a value in a nested map structure using a JSON Pointer path.
func setValueAtPath(root map[string]any, path string, value any) {
	if path == "" {
		return
	}

	parts, err := jsonptr.Parse(path)
	if err != nil || len(parts) == 0 {
		return
	}

	current := root
	for _, part := range parts[:len(parts)-1] {
		if next, ok := current[part]; ok {
			if nextMap, ok := next.(map[string]any); ok {
				current = nextMap
			} else {
				// Path conflict - create new map
				newMap := make(map[string]any)
				current[part] = newMap
				current = newMap
			}
		} else {
			// Create intermediate map
			newMap := make(map[string]any)
			current[part] = newMap
			current = newMap
		}
	}

	current[parts[len(parts)-1]] = value
}

// Set sets the value at the canonical path by transforming to source path.
func (d *TransformedDocument) Set(path string, value any) error {
	sourcePath := d.toSourcePath(path)
	return d.inner.Set(sourcePath, value)
}

// Delete removes the value at the canonical path by transforming to source path.
func (d *TransformedDocument) Delete(path string) error {
	sourcePath := d.toSourcePath(path)
	return d.inner.Delete(sourcePath)
}

func (d *TransformedDocument) Marshal() ([]byte, error) {
	return d.inner.Marshal()
}

func (d *TransformedDocument) Format() document.DocumentFormat {
	return d.inner.Format()
}

func (d *TransformedDocument) MarshalTestData(data map[string]any) ([]byte, error) {
	return d.inner.MarshalTestData(data)
}
