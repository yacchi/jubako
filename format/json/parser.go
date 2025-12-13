package json

import (
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/format"
)

// NewParser creates a new JSON parser.
//
// Note: JSON can be marshaled, but original formatting is not preserved.
//
// Example:
//
//	parser := json.NewParser()
//	layer := layer.New("config", fs.New("config.json"), parser)
func NewParser() document.Parser {
	return format.NewParser(document.FormatJSON, Parse, format.ParserConfig{})
}
