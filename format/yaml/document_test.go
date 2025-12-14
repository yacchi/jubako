package yaml

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
)

func TestNew(t *testing.T) {
	doc := New()
	if doc == nil {
		t.Fatal("New() returned nil")
	}

	if doc.Format() != document.FormatYAML {
		t.Errorf("Format() = %v, want %v", doc.Format(), document.FormatYAML)
	}
}

func TestDocument_Get(t *testing.T) {
	yamlData := []byte(`
server:
  host: localhost
  port: 8080
  enabled: true
  ratio: 0.75
database:
  connections:
    primary:
      host: db.example.com
      port: 5432
servers:
  - name: server1
    port: 8080
  - name: server2
    port: 8081
empty_map: {}
null_value: null
`)

	doc := New()

	data, err := doc.Get(yamlData)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Verify string value
	server, ok := data["server"].(map[string]any)
	if !ok {
		t.Fatal("data[server] is not a map")
	}
	if server["host"] != "localhost" {
		t.Errorf("server[host] = %v, want localhost", server["host"])
	}

	// Verify int value
	if server["port"] != 8080 {
		t.Errorf("server[port] = %v, want 8080", server["port"])
	}

	// Verify bool value
	if server["enabled"] != true {
		t.Errorf("server[enabled] = %v, want true", server["enabled"])
	}

	// Verify float value
	if server["ratio"] != 0.75 {
		t.Errorf("server[ratio] = %v, want 0.75", server["ratio"])
	}

	// Verify nested value
	database, ok := data["database"].(map[string]any)
	if !ok {
		t.Fatal("data[database] is not a map")
	}
	connections, ok := database["connections"].(map[string]any)
	if !ok {
		t.Fatal("data[database][connections] is not a map")
	}
	primary, ok := connections["primary"].(map[string]any)
	if !ok {
		t.Fatal("data[database][connections][primary] is not a map")
	}
	if primary["host"] != "db.example.com" {
		t.Errorf("primary[host] = %v, want db.example.com", primary["host"])
	}

	// Verify array
	servers, ok := data["servers"].([]any)
	if !ok {
		t.Fatal("data[servers] is not a slice")
	}
	if len(servers) != 2 {
		t.Errorf("len(servers) = %d, want 2", len(servers))
	}
	s1, ok := servers[0].(map[string]any)
	if !ok {
		t.Fatal("servers[0] is not a map")
	}
	if s1["name"] != "server1" {
		t.Errorf("servers[0][name] = %v, want server1", s1["name"])
	}

	// Verify empty map
	emptyMap, ok := data["empty_map"].(map[string]any)
	if !ok {
		t.Fatal("data[empty_map] is not a map")
	}
	if len(emptyMap) != 0 {
		t.Errorf("data[empty_map] is not empty: %v", emptyMap)
	}

	// Verify null value
	if _, ok := data["null_value"]; !ok {
		t.Error("data[null_value] key should exist")
	}
	if data["null_value"] != nil {
		t.Errorf("data[null_value] = %v, want nil", data["null_value"])
	}
}

func TestDocument_Get_Empty(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"empty string", ""},
		{"whitespace only", " \n\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := New()
			data, err := doc.Get([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if data == nil {
				t.Error("Get() returned nil")
			}
			if len(data) != 0 {
				t.Errorf("Get() returned non-empty map: %v", data)
			}
		})
	}
}

func TestDocument_Get_Invalid(t *testing.T) {
	invalidYAML := []byte(`
server: [
  host: localhost
  port: 8080
`)
	doc := New()

	_, err := doc.Get(invalidYAML)
	if err == nil {
		t.Error("Get() should return error for invalid YAML")
	}
}

func TestDocument_Format(t *testing.T) {
	doc := New()
	if doc.Format() != document.FormatYAML {
		t.Errorf("Format() = %v, want %v", doc.Format(), document.FormatYAML)
	}
}

func TestDocument_Apply(t *testing.T) {
	doc := New()

	input := []byte(`server:
  host: localhost
  port: 8080
`)

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
	if server["port"] != 8080 {
		t.Errorf("server.port = %v, want 8080", server["port"])
	}
}

func TestDocument_Apply_WithChangeset(t *testing.T) {
	input := []byte(`# Server configuration
server:
  host: localhost  # Default host
  port: 8080       # Default port
`)
	doc := New()

	changeset := document.JSONPatchSet{
		document.NewReplacePatch("/server/port", 9000),
	}

	out, err := doc.Apply(input, changeset)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	output := string(out)

	// Check that comments are preserved
	if !strings.Contains(output, "# Server configuration") {
		t.Error("Apply() did not preserve heading comment")
	}

	if !strings.Contains(output, "# Default host") {
		t.Error("Apply() did not preserve inline comment for host")
	}

	if !strings.Contains(output, "# Default port") {
		t.Error("Apply() did not preserve inline comment for port")
	}

	// Check that the value was updated
	if !strings.Contains(output, "9000") {
		t.Error("Apply() did not include updated port value")
	}
}

func TestDocument_Apply_AddValue(t *testing.T) {
	input := []byte(`server:
  host: localhost
`)
	doc := New()

	changeset := document.JSONPatchSet{
		document.NewAddPatch("/server/port", 8080),
	}

	out, err := doc.Apply(input, changeset)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Parse result and verify
	doc2 := New()
	result, err := doc2.Get(out)
	if err != nil {
		t.Fatalf("Get() after Apply() error = %v", err)
	}

	server, ok := result["server"].(map[string]any)
	if !ok {
		t.Fatal("result[server] is not a map")
	}
	if server["port"] != 8080 {
		t.Errorf("server.port = %v, want 8080", server["port"])
	}
}

func TestDocument_Apply_RemoveValue(t *testing.T) {
	input := []byte(`server:
  host: localhost
  port: 8080
`)
	doc := New()

	changeset := document.JSONPatchSet{
		document.NewRemovePatch("/server/port"),
	}

	out, err := doc.Apply(input, changeset)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Parse result and verify
	doc2 := New()
	result, err := doc2.Get(out)
	if err != nil {
		t.Fatalf("Get() after Apply() error = %v", err)
	}

	server, ok := result["server"].(map[string]any)
	if !ok {
		t.Fatal("result[server] is not a map")
	}
	if _, ok := server["port"]; ok {
		t.Error("server.port should not exist after remove")
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
	if result["count"] != 42 {
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

func TestDocument_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "simple document",
			yaml: `server:
  host: localhost
  port: 8080
`,
		},
		{
			name: "document with arrays",
			yaml: `servers:
  - name: server1
    port: 8080
  - name: server2
    port: 8081
`,
		},
		{
			name: "document with mixed types",
			yaml: `string: value
integer: 42
float: 3.14
bool: true
null_value: null
array:
  - item1
  - item2
object:
  key: value
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse
			doc1 := New()

			data1, err := doc1.Get([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}

			// Apply (marshal)
			out, err := doc1.Apply([]byte(tt.yaml), nil)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}

			// Parse again
			doc2 := New()
			data2, err := doc2.Get(out)
			if err != nil {
				t.Fatalf("Parse(Apply()) error = %v", err)
			}

			// Compare
			if !reflect.DeepEqual(data1, data2) {
				t.Errorf("Round-trip value mismatch:\noriginal: %v\nround-trip: %v", data1, data2)
			}
		})
	}
}

func TestDocument_Anchors(t *testing.T) {
	t.Run("get value through alias", func(t *testing.T) {
		yamlData := []byte(`
defaults: &defaults
  adapter: postgres
  host: localhost
  port: 5432

development:
  database: dev_db
  <<: *defaults

production:
  database: prod_db
  <<: *defaults
`)
		doc := New()
		data, err := doc.Get(yamlData)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		// Get value from anchor directly
		defaults, ok := data["defaults"].(map[string]any)
		if !ok {
			t.Fatal("data[defaults] is not a map")
		}
		if defaults["host"] != "localhost" {
			t.Errorf("defaults[host] = %v, want localhost", defaults["host"])
		}

		// YAML merge keys are handled during unmarshaling
		dev, ok := data["development"].(map[string]any)
		if !ok {
			t.Fatal("data[development] is not a map")
		}
		if dev["database"] != "dev_db" {
			t.Errorf("development[database] = %v, want dev_db", dev["database"])
		}
	})

	t.Run("get value from alias reference", func(t *testing.T) {
		yamlData := []byte(`
base: &base
  name: baseValue
  nested:
    key: nestedValue

ref: *base
`)
		doc := New()
		data, err := doc.Get(yamlData)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		ref, ok := data["ref"].(map[string]any)
		if !ok {
			t.Fatal("data[ref] is not a map")
		}
		if ref["name"] != "baseValue" {
			t.Errorf("ref[name] = %v, want baseValue", ref["name"])
		}

		nested, ok := ref["nested"].(map[string]any)
		if !ok {
			t.Fatal("ref[nested] is not a map")
		}
		if nested["key"] != "nestedValue" {
			t.Errorf("ref[nested][key] = %v, want nestedValue", nested["key"])
		}
	})

	t.Run("scalar alias", func(t *testing.T) {
		yamlData := []byte(`
default_port: &port 8080
server:
  port: *port
`)
		doc := New()
		data, err := doc.Get(yamlData)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		server, ok := data["server"].(map[string]any)
		if !ok {
			t.Fatal("data[server] is not a map")
		}
		if server["port"] != 8080 {
			t.Errorf("server[port] = %v, want 8080", server["port"])
		}
	})
}

func TestDocument_Interface(t *testing.T) {
	var _ document.Document = (*Document)(nil)
	var _ document.Document = New()
}

// TestDocument_Compliance runs the standard jktest compliance tests.
func TestDocument_Compliance(t *testing.T) {
	factory := jktest.DocumentLayerFactory(New())
	jktest.NewLayerTester(t, factory).TestAll()
}
