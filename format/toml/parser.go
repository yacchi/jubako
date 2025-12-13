package toml

import (
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/format"
)

// NewParser creates a new TOML parser.
//
// Example:
//
//	parser := toml.NewParser()
//	layer := layer.New("config", fs.New("config.toml"), parser)
func NewParser() document.Parser {
	return format.NewParser(document.FormatTOML, Parse, format.ParserConfig{})
}
