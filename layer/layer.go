// Package layer provides interfaces and implementations for configuration layers.
// A layer represents a single configuration source with a priority.
// Layers are merged in order of priority to produce the final configuration.
package layer

import (
	"context"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/source"
)

// Priority defines the priority of a configuration layer.
// Higher values take precedence during merging.
type Priority int

// Name is a unique identifier for a configuration layer.
type Name string

// Layer represents a configuration source that can be loaded and optionally saved.
// Implementations handle the details of loading from various sources (files, environment, etc.).
// Priority is not part of the Layer interface - it's managed by the Store when adding layers.
type Layer interface {
	// Name returns the unique identifier for this layer.
	Name() Name

	// Load reads configuration from the source and returns data as map[string]any.
	// The context can be used for cancellation and timeouts.
	Load(ctx context.Context) (map[string]any, error)

	// Save persists data back to the source using optimistic locking.
	// The changeset contains modifications since last load (for comment-preserving formats).
	// Returns source.ErrSaveNotSupported if the layer doesn't support saving.
	// Returns source.ErrSourceModified if the source was modified externally.
	Save(ctx context.Context, changeset document.JSONPatchSet) error

	// CanSave returns true if this layer supports saving.
	CanSave() bool
}

// FileLayer is a Layer implementation that combines a Source and Document.
// This is the standard layer type for file-based configurations (YAML, TOML, JSON, etc.).
type FileLayer struct {
	name   Name
	source source.Source
	doc    document.Document
}

// FormatProvider is an optional interface that layers can implement
// to expose their document format. This is useful for introspection
// and debugging purposes.
type FormatProvider interface {
	Format() document.DocumentFormat
}

// Ensure FileLayer implements Layer interface.
var _ Layer = (*FileLayer)(nil)

// Ensure FileLayer implements FormatProvider interface.
var _ FormatProvider = (*FileLayer)(nil)

// New creates a new FileLayer with the given Source and Document.
// The Document is stateless and handles format parsing/serialization.
//
// Example:
//
//	src := fs.New("~/.config/app.yaml")
//	layer := layer.New("user", src, yaml.New())
func New(name Name, src source.Source, doc document.Document) *FileLayer {
	return &FileLayer{
		name:   name,
		source: src,
		doc:    doc,
	}
}

// Name returns the layer's name.
func (l *FileLayer) Name() Name {
	return l.name
}

// Load reads from the source via Document.Get and returns data as map[string]any.
func (l *FileLayer) Load(ctx context.Context) (map[string]any, error) {
	data, err := l.source.Load(ctx)
	if err != nil {
		return nil, err
	}
	return l.doc.Get(data)
}

// Save generates output via Document.Apply and saves to the source.
// Uses optimistic locking to detect external modifications.
// Returns source.ErrSaveNotSupported if the source doesn't support saving.
// Returns source.ErrSourceModified if the source was modified externally.
func (l *FileLayer) Save(ctx context.Context, changeset document.JSONPatchSet) error {
	return l.source.Save(ctx, func(current []byte) ([]byte, error) {
		return l.doc.Apply(current, changeset)
	})
}

// Source returns the underlying source (useful for accessing source-specific methods).
func (l *FileLayer) Source() source.Source {
	return l.source
}

// Document returns the underlying document.
func (l *FileLayer) Document() document.Document {
	return l.doc
}

// CanSave returns true if this layer supports saving.
// The source must support saving for this to return true.
func (l *FileLayer) CanSave() bool {
	return l.source.CanSave()
}

// Format returns the document format (e.g., "yaml", "toml", "json").
func (l *FileLayer) Format() document.DocumentFormat {
	return l.doc.Format()
}
