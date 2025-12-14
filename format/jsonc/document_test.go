package jsonc

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yacchi/jubako/document"
)

func TestDocument_Get(t *testing.T) {
	doc := New()

	data, err := doc.Get([]byte(`{"a":1,"b":{"c":true},"items":["x","y"]}`))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Check values
	if v, ok := data["a"]; !ok || v != float64(1) {
		t.Errorf("data[a] = %v, want 1", v)
	}

	b, ok := data["b"].(map[string]any)
	if !ok {
		t.Fatalf("data[b] is not a map")
	}
	if b["c"] != true {
		t.Errorf("data[b][c] = %v, want true", b["c"])
	}

	items, ok := data["items"].([]any)
	if !ok {
		t.Fatalf("data[items] is not a slice")
	}
	if items[0] != "x" {
		t.Errorf("data[items][0] = %v, want x", items[0])
	}
}

func TestDocument_Get_WithComments(t *testing.T) {
	doc := New()

	data, err := doc.Get([]byte(`{
  // comment
  "a": 1,
  "b": {"c": true},
  "items": ["x", "y",],
}
`))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if v, ok := data["a"]; !ok || v != float64(1) {
		t.Errorf("data[a] = %v, want 1", v)
	}
	if b, ok := data["b"].(map[string]any); !ok || b["c"] != true {
		t.Errorf("data[b][c] != true")
	}
	if items, ok := data["items"].([]any); !ok || items[0] != "x" {
		t.Errorf("data[items][0] != x")
	}
}

func TestDocument_Get_Empty(t *testing.T) {
	doc := New()

	data, err := doc.Get([]byte(" \n\t"))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if data == nil {
		t.Error("Get() returned nil for empty document")
	}
	if len(data) != 0 {
		t.Errorf("Get() returned non-empty map for empty document: %v", data)
	}
}

func TestDocument_Format(t *testing.T) {
	doc := New()
	if doc.Format() != document.FormatJSONC {
		t.Errorf("Format() = %v, want %v", doc.Format(), document.FormatJSONC)
	}
}

func TestDocument_Apply(t *testing.T) {
	doc := New()

	input := []byte(`{
  "server": {
    "host": "localhost",
    "port": 8080
  }
}`)

	data, err := doc.Apply(input, nil)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Apply() returned empty data")
	}

	// Parse back and verify
	doc2 := New()
	result, err := doc2.Get(data)
	if err != nil {
		t.Fatalf("Get() after Apply() error = %v", err)
	}

	server, ok := result["server"].(map[string]any)
	if !ok {
		t.Fatal("result[server] is not a map")
	}
	if server["host"] != "localhost" {
		t.Errorf("server.host = %v, want localhost", server["host"])
	}
	if server["port"] != float64(8080) {
		t.Errorf("server.port = %v, want 8080", server["port"])
	}
}

func TestDocument_Apply_WithChangeset(t *testing.T) {
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

func TestDocument_MarshalTestData(t *testing.T) {
	doc := New()

	testData := map[string]any{
		"key":   "value",
		"count": 42,
	}

	data, err := doc.MarshalTestData(testData)
	if err != nil {
		t.Fatalf("MarshalTestData() error = %v", err)
	}

	// Parse back and verify
	doc2 := New()
	result, err := doc2.Get(data)
	if err != nil {
		t.Fatalf("Get() after MarshalTestData() error = %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("result[key] = %v, want value", result["key"])
	}
	if result["count"] != float64(42) {
		t.Errorf("result[count] = %v, want 42", result["count"])
	}
}

func TestDocument_Get_NilData(t *testing.T) {
	doc := New()
	data, err := doc.Get(nil)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(data, map[string]any{}) {
		t.Errorf("Get() with nil data = %v, want empty map", data)
	}
}
