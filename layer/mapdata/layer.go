// Package mapdata provides a map-based configuration layer.
// This layer is useful for testing and programmatic configuration injection
// without requiring any format-specific dependencies (YAML, TOML, etc.).
//
// The layer operates directly on map[string]any data structures, making it
// ideal for unit tests where you want to inject configuration without parsing.
package mapdata

import (
	"context"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/mapdoc"
)

// Layer is a configuration layer backed by a map[string]any.
// It supports read/write operations and is useful for testing and
// programmatic configuration.
type Layer struct {
	name layer.Name
	data map[string]any
	doc  *Document
}

// Ensure Layer implements layer.Layer interface.
var _ layer.Layer = (*Layer)(nil)

// New creates a new mapdata layer with the given data.
// The data map is used directly (not copied), so modifications to the
// original map will affect the layer until Load() is called.
//
// Example:
//
//	data := map[string]any{
//	    "server": map[string]any{
//	        "host": "localhost",
//	        "port": 8080,
//	    },
//	}
//	layer := mapdata.New("test", data)
func New(name layer.Name, data map[string]any) *Layer {
	if data == nil {
		data = make(map[string]any)
	}
	return &Layer{
		name: name,
		data: data,
	}
}

// Name returns the layer's name.
func (l *Layer) Name() layer.Name {
	return l.name
}

// Load creates a Document from the layer's data.
// This deep-copies the data to prevent external modifications.
func (l *Layer) Load(ctx context.Context) (document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Deep copy the data to prevent external modifications
	l.doc = NewDocument(deepCopyMap(l.data))
	return l.doc, nil
}

// Document returns the last loaded document.
func (l *Layer) Document() document.Document {
	if l.doc == nil {
		return nil
	}
	return l.doc
}

// Save is a no-op for mapdata layers.
// The document is in-memory, so changes are immediately available via Data().
// This method exists to satisfy the layer.Layer interface and allow SetTo operations.
func (l *Layer) Save(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// No-op: data is already in memory
	return nil
}

// CanSave returns true because mapdata layers support in-memory modifications.
// Note: Changes are not persisted to any storage; use Data() to access modified data.
func (l *Layer) CanSave() bool {
	return true
}

// Data returns the current document data.
// Returns nil if the layer hasn't been loaded yet.
func (l *Layer) Data() map[string]any {
	if l.doc == nil {
		return nil
	}
	return l.doc.Data()
}

// Document is a document.Document implementation backed by map[string]any.
// It is a thin wrapper around mapdoc.Document configured for the "mapdata" format.
type Document = mapdoc.Document

// Ensure Document implements document.Document interface.
var _ document.Document = (*Document)(nil)

// NewDocument creates a new Document with the given data.
func NewDocument(data map[string]any) *Document {
	return mapdoc.New("mapdata", mapdoc.WithData(data))
}

// deepCopyMap creates a deep copy of a map[string]any.
func deepCopyMap(src map[string]any) map[string]any {
	return mapdoc.DeepCopyMap(src)
}
