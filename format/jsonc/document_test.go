package jsonc

import (
	"strings"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
)

func TestDocument_Format(t *testing.T) {
	doc := New()
	if doc.Format() != document.FormatJSONC {
		t.Errorf("Format() = %v, want %v", doc.Format(), document.FormatJSONC)
	}
}

// TestDocument_Apply_CommentPreservation verifies JSONC-specific comment preservation.
func TestDocument_Apply_CommentPreservation(t *testing.T) {
	input := []byte(`{
  // heading
  "server": {
    "host": "localhost" // inline
  }
}
`)
	doc := New()

	changeset := document.JSONPatchSet{
		document.NewAddPatch("/server/port", 9000),
	}

	out, err := doc.Apply(input, changeset)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "// heading") {
		t.Error("Apply() did not preserve heading comment")
	}
	if !strings.Contains(s, "inline") {
		t.Error("Apply() did not preserve inline comment")
	}
	if !strings.Contains(s, "port") || !strings.Contains(s, "9000") {
		t.Error("Apply() did not include updated value")
	}
}

// TestDocument_Compliance runs the standard jktest compliance tests.
func TestDocument_Compliance(t *testing.T) {
	factory := jktest.DocumentLayerFactory(New())
	jktest.NewLayerTester(t, factory).TestAll()
}
