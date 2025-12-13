package yaml

import (
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/format"
)

// NewParser creates a new YAML parser.
//
// Example:
//
//	parser := yaml.NewParser()
//	layer := layer.New("config", fs.New("config.yaml"), parser)
func NewParser() document.Parser {
	return format.NewParser(document.FormatYAML, Parse, format.ParserConfig{})
}
