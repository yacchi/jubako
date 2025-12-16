package json

import (
	"strings"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
)

// TestDocument_Compliance runs the standard jktest compliance tests.
func TestDocument_Compliance(t *testing.T) {
	jktest.NewDocumentLayerTester(t, New()).TestAll()
}

func TestDocument_Get_InvalidJSON(t *testing.T) {
	doc := New()
	_, err := doc.Get([]byte("{ invalid"))
	if err == nil {
		t.Fatal("Get() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse JSON") {
		t.Fatalf("error = %q, want to contain %q", err.Error(), "failed to parse JSON")
	}
}

func TestDocument_Apply_InvalidJSON(t *testing.T) {
	doc := New()
	_, err := doc.Apply([]byte("{ invalid"), nil)
	if err == nil {
		t.Fatal("Apply() expected error, got nil")
	}
}

func TestDocument_MarshalTestData_Error(t *testing.T) {
	doc := New()
	_, err := doc.MarshalTestData(map[string]any{"x": func() {}})
	if err == nil {
		t.Fatal("MarshalTestData() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to marshal JSON test data") {
		t.Fatalf("error = %q, want to contain %q", err.Error(), "failed to marshal JSON test data")
	}
}

func TestDocument_Apply_AddsNewline(t *testing.T) {
	doc := New()
	var patches document.JSONPatchSet
	patches.Add("/a", 1)
	out, err := doc.Apply(nil, patches)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(out) == 0 || out[len(out)-1] != '\n' {
		t.Fatalf("Apply() output should end with newline: %q", string(out))
	}
}

func TestDocument_Apply_MarshalError(t *testing.T) {
	doc := New()
	var patches document.JSONPatchSet
	patches.Add("/bad", func() {})

	_, err := doc.Apply([]byte("{}\n"), patches)
	if err == nil {
		t.Fatal("Apply() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to marshal JSON") {
		t.Fatalf("error = %q, want to contain %q", err.Error(), "failed to marshal JSON")
	}
}
