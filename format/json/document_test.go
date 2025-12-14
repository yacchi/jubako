package json

import (
	"reflect"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
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
	if doc.Format() != document.FormatJSON {
		t.Errorf("Format() = %v, want %v", doc.Format(), document.FormatJSON)
	}
}

func TestDocument_Apply(t *testing.T) {
	doc := New()

	current := []byte(`{"server":{"host":"localhost","port":8080}}`)

	data, err := doc.Apply(current, nil)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Apply() returned empty data")
	}

	// Parse back and verify
	result, err := doc.Get(data)
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
	doc := New()

	current := []byte(`{"server":{"host":"localhost","port":8080}}`)

	changeset := document.JSONPatchSet{
		document.NewReplacePatch("/server/port", 9000),
		document.NewAddPatch("/server/timeout", 30),
	}

	data, err := doc.Apply(current, changeset)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Parse back and verify
	result, err := doc.Get(data)
	if err != nil {
		t.Fatalf("Get() after Apply() error = %v", err)
	}

	server, ok := result["server"].(map[string]any)
	if !ok {
		t.Fatal("result[server] is not a map")
	}
	if server["port"] != float64(9000) {
		t.Errorf("server.port = %v, want 9000", server["port"])
	}
	if server["timeout"] != float64(30) {
		t.Errorf("server.timeout = %v, want 30", server["timeout"])
	}
}

func TestDocument_Apply_Remove(t *testing.T) {
	doc := New()

	current := []byte(`{"server":{"host":"localhost","port":8080}}`)

	changeset := document.JSONPatchSet{
		document.NewRemovePatch("/server/port"),
	}

	data, err := doc.Apply(current, changeset)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Parse back and verify
	result, err := doc.Get(data)
	if err != nil {
		t.Fatalf("Get() after Apply() error = %v", err)
	}

	server, ok := result["server"].(map[string]any)
	if !ok {
		t.Fatal("result[server] is not a map")
	}
	if _, exists := server["port"]; exists {
		t.Error("server.port should not exist after remove")
	}
	if server["host"] != "localhost" {
		t.Errorf("server.host = %v, want localhost", server["host"])
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
	result, err := doc.Get(data)
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

// TestDocument_Compliance runs the standard jktest compliance tests.
func TestDocument_Compliance(t *testing.T) {
	factory := jktest.DocumentLayerFactory(New())
	jktest.NewLayerTester(t, factory).TestAll()
}
