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

	// Load reads configuration from the source and returns a Document.
	// The context can be used for cancellation and timeouts.
	Load(ctx context.Context) (document.Document, error)

	// Document returns the last loaded Document, or nil if not loaded.
	Document() document.Document

	// Save persists the current document back to the source.
	// Returns source.ErrSaveNotSupported if the layer doesn't support saving.
	Save(ctx context.Context) error

	// CanSave returns true if this layer supports saving.
	// This checks both the source and the document format.
	CanSave() bool
}

// SourceParser is a Layer implementation that combines a Source and Parser.
// This is the standard layer type for file-based configurations (YAML, TOML, JSON, etc.).
type SourceParser struct {
	name   Name
	source source.Source
	parser document.Parser
	doc    document.Document
}

// Ensure SourceParser implements Layer interface.
var _ Layer = (*SourceParser)(nil)

// New creates a new SourceParser layer with the given Source and Parser.
//
// Example:
//
//	layer := layer.New("user", fs.New("~/.config/app.yaml"), yaml.NewParser())
func New(name Name, src source.Source, parser document.Parser) *SourceParser {
	return &SourceParser{
		name:   name,
		source: src,
		parser: parser,
	}
}

// Name returns the layer's name.
func (l *SourceParser) Name() Name {
	return l.name
}

// Load reads from the source and parses into a Document.
func (l *SourceParser) Load(ctx context.Context) (document.Document, error) {
	data, err := l.source.Load(ctx)
	if err != nil {
		return nil, err
	}

	doc, err := l.parser.Parse(data)
	if err != nil {
		return nil, err
	}

	l.doc = doc
	return doc, nil
}

// Save marshals the document and saves to the source.
// Returns source.ErrSaveNotSupported if the source doesn't support saving.
func (l *SourceParser) Save(ctx context.Context) error {
	if l.doc == nil {
		return nil
	}

	data, err := l.doc.Marshal()
	if err != nil {
		return err
	}

	return l.source.Save(ctx, data)
}

// Document returns the last loaded document.
func (l *SourceParser) Document() document.Document {
	return l.doc
}

// Source returns the underlying source (useful for accessing source-specific methods).
func (l *SourceParser) Source() source.Source {
	return l.source
}

// Parser returns the underlying parser.
func (l *SourceParser) Parser() document.Parser {
	return l.parser
}

// CanSave returns true if this layer supports saving.
// Both the source and parser must support saving/marshaling for this to return true.
func (l *SourceParser) CanSave() bool {
	return l.source.CanSave() && l.parser.CanMarshal()
}
