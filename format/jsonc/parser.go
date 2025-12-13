package jsonc

import (
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/format"
)

// NewParser creates a new JSONC parser.
//
// Example:
//
//	parser := jsonc.NewParser()
//	layer := layer.New("config", fs.New("config.jsonc"), parser)
func NewParser() document.Parser {
	return format.NewParser(document.FormatJSONC, Parse, format.ParserConfig{})
}
