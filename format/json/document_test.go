package json

import (
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
)

func TestDocument_Format(t *testing.T) {
	doc := New()
	if doc.Format() != document.FormatJSON {
		t.Errorf("Format() = %v, want %v", doc.Format(), document.FormatJSON)
	}
}

// TestDocument_Compliance runs the standard jktest compliance tests.
func TestDocument_Compliance(t *testing.T) {
	factory := jktest.DocumentLayerFactory(New())
	jktest.NewLayerTester(t, factory).TestAll()
}
