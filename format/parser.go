// Package format provides common utilities for document format implementations.
package format

import "github.com/yacchi/jubako/document"

// ParseFunc is a function that parses bytes into a Document.
type ParseFunc func([]byte) (document.Document, error)

// ParserConfig configures optional parser behavior.
type ParserConfig struct {
	// CanMarshal indicates whether documents can be marshaled with comment preservation.
	// Default: true
	CanMarshal *bool
}

// NewParser creates a Parser with the given format and parse function.
//
// The format and parse arguments are required. Optional configuration
// can be provided via ParserConfig.
//
// Example:
//
//	// Basic usage
//	parser := format.NewParser(document.FormatYAML, yaml.Parse, format.ParserConfig{})
//
//	// With options
//	parser := format.NewParser(document.FormatJSON, json.Parse, format.ParserConfig{
//	    CanMarshal: format.Ptr(false),
//	})
func NewParser(fmt document.DocumentFormat, parse ParseFunc, cfg ParserConfig) document.Parser {
	canMarshal := true
	if cfg.CanMarshal != nil {
		canMarshal = *cfg.CanMarshal
	}

	return &parser{
		format:     fmt,
		parseFunc:  parse,
		canMarshal: canMarshal,
	}
}

// parser implements document.Parser using the provided configuration.
type parser struct {
	format     document.DocumentFormat
	parseFunc  ParseFunc
	canMarshal bool
}

// Ensure parser implements the document.Parser interface.
var _ document.Parser = (*parser)(nil)

// Parse implements the document.Parser interface.
func (p *parser) Parse(data []byte) (document.Document, error) {
	return p.parseFunc(data)
}

// Format implements the document.Parser interface.
func (p *parser) Format() document.DocumentFormat {
	return p.format
}

// CanMarshal implements the document.Parser interface.
func (p *parser) CanMarshal() bool {
	return p.canMarshal
}

// MarshalTestData implements the document.Parser interface.
func (p *parser) MarshalTestData(data map[string]any) ([]byte, error) {
	doc, err := p.parseFunc(nil)
	if err != nil {
		return nil, err
	}
	return doc.MarshalTestData(data)
}

// Ptr returns a pointer to the given value.
// This is a helper for setting optional fields in ParserConfig.
//
// Example:
//
//	parser := format.NewParser(document.FormatJSON, json.Parse, format.ParserConfig{
//	    CanMarshal: format.Ptr(false),
//	})
func Ptr[T any](v T) *T {
	return &v
}
