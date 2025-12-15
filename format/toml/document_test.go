package toml

import (
	"errors"
	"strings"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
)

func TestDocument_Format(t *testing.T) {
	doc := New()
	if doc.Format() != document.FormatTOML {
		t.Errorf("Format() = %v, want %v", doc.Format(), document.FormatTOML)
	}
}

// TestDocument_Apply_CommentPreservation verifies TOML-specific comment preservation.
func TestDocument_Apply_CommentPreservation(t *testing.T) {
	input := []byte("# heading\n[server]\nhost = \"localhost\" # inline\nport = 8080\n")
	doc := New()

	changeset := document.JSONPatchSet{
		document.NewReplacePatch("/server/port", int64(9000)),
	}

	out, err := doc.Apply(input, changeset)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "# heading") {
		t.Error("Apply() did not preserve heading comment")
	}
	if !strings.Contains(s, "host = \"localhost\" # inline") {
		t.Error("Apply() did not preserve inline comment")
	}
	if !strings.Contains(s, "port = 9000") {
		t.Error("Apply() did not include updated value")
	}
}

func TestDocument_MarshalTestData_NullValue(t *testing.T) {
	doc := New()

	testData := map[string]any{
		"key":  "value",
		"null": nil,
	}

	_, err := doc.MarshalTestData(testData)
	if err == nil {
		t.Error("MarshalTestData() should return error for null values")
	}

	var unsupportedErr *document.UnsupportedStructureError
	if !errors.As(err, &unsupportedErr) {
		t.Errorf("MarshalTestData() error type = %T, want *document.UnsupportedStructureError", err)
	}
}

// TestDocument_Compliance runs the standard jktest compliance tests.
func TestDocument_Compliance(t *testing.T) {
	factory := jktest.DocumentLayerFactory(New())
	jktest.NewLayerTester(t, factory,
		jktest.SkipNullTest("TOML format does not support null values"),
	).TestAll()
}
