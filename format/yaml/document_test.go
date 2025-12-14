package yaml

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
)

func TestDocument_Format(t *testing.T) {
	doc := New()
	if doc.Format() != document.FormatYAML {
		t.Errorf("Format() = %v, want %v", doc.Format(), document.FormatYAML)
	}
}

// TestDocument_Get_Invalid verifies error handling for invalid YAML.
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

// TestDocument_Apply_CommentPreservation verifies YAML-specific comment preservation.
func TestDocument_Apply_CommentPreservation(t *testing.T) {
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

// TestDocument_RoundTrip verifies YAML parsing and marshaling produce consistent results.
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
