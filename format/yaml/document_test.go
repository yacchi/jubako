package yaml

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
	"gopkg.in/yaml.v3"
)


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
	jktest.NewDocumentLayerTester(t, New()).TestAll()
}

func TestGet_EmptyInput(t *testing.T) {
	doc := New()
	got, err := doc.Get(nil)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("Get(nil) = %#v, want empty map", got)
	}
}

func TestGet_NullDocument_ReturnsEmptyMap(t *testing.T) {
	doc := New()
	got, err := doc.Get([]byte("null\n"))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("Get(null) = %#v, want empty map", got)
	}
}

func TestApply_InvalidYAML_WithChangeset_CreatesNewDocument(t *testing.T) {
	doc := New()

	out, err := doc.Apply([]byte("this: [\n"), document.JSONPatchSet{
		document.NewAddPatch("/a", 1),
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	parsed, err := doc.Get(out)
	if err != nil {
		t.Fatalf("Get(Apply()) error = %v", err)
	}
	if parsed["a"] != 1 {
		t.Fatalf("parsed[a] = %#v, want 1", parsed["a"])
	}
}

func TestApply_EmptyInputWithChangeset_CreatesNewDocument(t *testing.T) {
	doc := New()
	out, err := doc.Apply(nil, document.JSONPatchSet{document.NewAddPatch("/a", 1)})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	parsed, err := doc.Get(out)
	if err != nil {
		t.Fatalf("Get(output) error = %v", err)
	}
	if parsed["a"] != 1 {
		t.Fatalf("parsed[a] = %#v, want 1", parsed["a"])
	}
}

func TestGetRootMapping_CreatesMappingForEmptyDocument(t *testing.T) {
	root := &yaml.Node{Kind: yaml.DocumentNode}
	m := getRootMapping(root)
	if m == nil {
		t.Fatal("getRootMapping() returned nil")
	}
	if m.Kind != yaml.MappingNode {
		t.Fatalf("kind = %v, want MappingNode", m.Kind)
	}
}

func TestGetRootMapping_NilAndWithContent(t *testing.T) {
	if got := getRootMapping(nil); got != nil {
		t.Fatalf("getRootMapping(nil) = %#v, want nil", got)
	}
	root := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{{Kind: yaml.MappingNode}},
	}
	if got := getRootMapping(root); got != root.Content[0] {
		t.Fatal("getRootMapping() did not return the document content node")
	}
}

func TestGetRootMapping_NonDocumentNode_ReturnsInput(t *testing.T) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	if got := getRootMapping(root); got != root {
		t.Fatal("getRootMapping() did not return the input node")
	}
}

func TestResolveAlias(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := resolveAlias(nil); got != nil {
			t.Fatalf("resolveAlias(nil) = %#v, want nil", got)
		}
	})

	t.Run("alias", func(t *testing.T) {
		target := &yaml.Node{Kind: yaml.ScalarNode, Value: "x"}
		alias := &yaml.Node{Kind: yaml.AliasNode, Alias: target}
		if got := resolveAlias(alias); got != target {
			t.Fatalf("resolveAlias(alias) = %#v, want %#v", got, target)
		}
	})
}

func TestNodeKindString(t *testing.T) {
	tests := []struct {
		kind yaml.Kind
		want string
	}{
		{yaml.DocumentNode, "document"},
		{yaml.SequenceNode, "sequence"},
		{yaml.MappingNode, "mapping"},
		{yaml.ScalarNode, "scalar"},
		{yaml.AliasNode, "alias"},
		{yaml.Kind(999), "unknown"},
	}
	for _, tt := range tests {
		if got := nodeKindString(tt.kind); got != tt.want {
			t.Fatalf("nodeKindString(%v) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestUpdateNodeValue_Types(t *testing.T) {
	n := &yaml.Node{}

	updateNodeValue(n, "s")
	if n.Kind != yaml.ScalarNode || n.Tag != "!!str" || n.Value != "s" {
		t.Fatalf("string update = %#v", n)
	}

	updateNodeValue(n, int64(2))
	if n.Tag != "!!int" || n.Value != "2" {
		t.Fatalf("int64 update = %#v", n)
	}

	updateNodeValue(n, float64(1.5))
	if n.Tag != "!!float" {
		t.Fatalf("float update = %#v", n)
	}

	updateNodeValue(n, true)
	if n.Tag != "!!bool" || n.Value != "true" {
		t.Fatalf("bool update = %#v", n)
	}

	updateNodeValue(n, nil)
	if n.Tag != "!!null" {
		t.Fatalf("nil update = %#v", n)
	}

	updateNodeValue(n, []any{"a", 1})
	if n.Kind != yaml.SequenceNode || len(n.Content) != 2 {
		t.Fatalf("slice update = %#v", n)
	}

	updateNodeValue(n, map[string]any{"a": 1})
	if n.Kind != yaml.MappingNode || len(n.Content) != 2 {
		t.Fatalf("map update = %#v", n)
	}

	updateNodeValue(n, struct{ A int }{A: 1})
	if n.Kind == 0 {
		t.Fatalf("default update produced empty node: %#v", n)
	}
}

func TestSetNodeValue_ErrorsAndPaths(t *testing.T) {
	t.Run("invalid path", func(t *testing.T) {
		if err := setNodeValue(&yaml.Node{Kind: yaml.MappingNode}, nil, 1); err == nil {
			t.Fatal("setNodeValue(nil keys) expected error, got nil")
		}
	})

	t.Run("mapping create nested and replace", func(t *testing.T) {
		root := &yaml.Node{Kind: yaml.MappingNode}
		if err := setNodeValue(root, []string{"a", "b"}, 1); err != nil {
			t.Fatalf("setNodeValue(create) error = %v", err)
		}
		if err := setNodeValue(root, []string{"a", "b"}, 2); err != nil {
			t.Fatalf("setNodeValue(replace) error = %v", err)
		}
	})

	t.Run("mapping converts scalar to mapping for deeper paths", func(t *testing.T) {
		root := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "a"},
				{Kind: yaml.ScalarNode, Value: "x"},
			},
		}
		if err := setNodeValue(root, []string{"a", "b"}, 1); err != nil {
			t.Fatalf("setNodeValue() error = %v", err)
		}
	})

	t.Run("mapping creates sequence when next key is numeric", func(t *testing.T) {
		root := &yaml.Node{Kind: yaml.MappingNode}
		if err := setNodeValue(root, []string{"a", "0"}, "x"); err != nil {
			t.Fatalf("setNodeValue() error = %v", err)
		}
	})

	t.Run("sequence append and nested creation", func(t *testing.T) {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		if err := setNodeValue(seq, []string{"0"}, "x"); err != nil {
			t.Fatalf("append leaf error = %v", err)
		}
		if err := setNodeValue(seq, []string{"1", "0"}, "y"); err != nil {
			t.Fatalf("append nested error = %v", err)
		}
	})

	t.Run("sequence append creates mapping when next key is not numeric", func(t *testing.T) {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		if err := setNodeValue(seq, []string{"0", "a"}, 1); err != nil {
			t.Fatalf("setNodeValue() error = %v", err)
		}
		if len(seq.Content) != 1 || seq.Content[0].Kind != yaml.MappingNode {
			t.Fatalf("seq = %#v, want mapping at index 0", seq)
		}
	})

	t.Run("sequence updates existing element", func(t *testing.T) {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "x"}}}
		if err := setNodeValue(seq, []string{"0"}, "y"); err != nil {
			t.Fatalf("update existing error = %v", err)
		}
	})

	t.Run("sequence converts scalar to mapping for deeper paths", func(t *testing.T) {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "x"}}}
		if err := setNodeValue(seq, []string{"0", "a"}, 1); err != nil {
			t.Fatalf("setNodeValue() error = %v", err)
		}
	})

	t.Run("sequence index errors", func(t *testing.T) {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		var ip *document.InvalidPathError
		if err := setNodeValue(seq, []string{"x"}, 1); !errors.As(err, &ip) {
			t.Fatalf("expected InvalidPathError, got %T", err)
		}
		if err := setNodeValue(seq, []string{"1"}, 1); !errors.As(err, &ip) {
			t.Fatalf("expected InvalidPathError, got %T", err)
		}
	})

	t.Run("type mismatch on scalar", func(t *testing.T) {
		scalar := &yaml.Node{Kind: yaml.ScalarNode}
		var tm *document.TypeMismatchError
		if err := setNodeValue(scalar, []string{"a"}, 1); !errors.As(err, &tm) {
			t.Fatalf("expected TypeMismatchError, got %T", err)
		}
	})
}

func TestSetNodeValue_UpdatesAliasTarget(t *testing.T) {
	target := &yaml.Node{Kind: yaml.ScalarNode, Value: "x"}
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "a"},
			{Kind: yaml.AliasNode, Alias: target},
		},
	}
	if err := setNodeValue(root, []string{"a"}, "y"); err != nil {
		t.Fatalf("setNodeValue() error = %v", err)
	}
	if target.Value != "y" {
		t.Fatalf("alias target = %q, want %q", target.Value, "y")
	}
}

func TestDeleteNode(t *testing.T) {
	t.Run("invalid path", func(t *testing.T) {
		if err := deleteNode(&yaml.Node{Kind: yaml.MappingNode}, nil); err == nil {
			t.Fatal("deleteNode(nil keys) expected error, got nil")
		}
	})

	t.Run("mapping delete", func(t *testing.T) {
		root := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "a"},
				{Kind: yaml.ScalarNode, Value: "x"},
			},
		}
		if err := deleteNode(root, []string{"a"}); err != nil {
			t.Fatalf("deleteNode() error = %v", err)
		}
		if len(root.Content) != 0 {
			t.Fatalf("after delete, content = %#v", root.Content)
		}
	})

	t.Run("mapping missing key is idempotent", func(t *testing.T) {
		root := &yaml.Node{Kind: yaml.MappingNode}
		if err := deleteNode(root, []string{"missing"}); err != nil {
			t.Fatalf("deleteNode() error = %v", err)
		}
	})

	t.Run("mapping delete nested key", func(t *testing.T) {
		root := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "a"},
				{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{Kind: yaml.ScalarNode, Value: "b"},
						{Kind: yaml.ScalarNode, Value: "x"},
					},
				},
			},
		}
		if err := deleteNode(root, []string{"a", "b"}); err != nil {
			t.Fatalf("deleteNode() error = %v", err)
		}
	})

	t.Run("sequence delete and idempotency", func(t *testing.T) {
		root := &yaml.Node{
			Kind:    yaml.SequenceNode,
			Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "x"}},
		}
		if err := deleteNode(root, []string{"0"}); err != nil {
			t.Fatalf("deleteNode(seq) error = %v", err)
		}
		if err := deleteNode(root, []string{"0"}); err != nil {
			t.Fatalf("deleteNode(seq missing) error = %v", err)
		}
	})

	t.Run("sequence invalid index is no-op", func(t *testing.T) {
		root := &yaml.Node{Kind: yaml.SequenceNode}
		if err := deleteNode(root, []string{"x"}); err != nil {
			t.Fatalf("deleteNode(seq invalid) error = %v", err)
		}
	})

	t.Run("sequence delete nested", func(t *testing.T) {
		root := &yaml.Node{
			Kind: yaml.SequenceNode,
			Content: []*yaml.Node{
				{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{Kind: yaml.ScalarNode, Value: "a"},
						{Kind: yaml.ScalarNode, Value: "x"},
					},
				},
			},
		}
		if err := deleteNode(root, []string{"0", "a"}); err != nil {
			t.Fatalf("deleteNode(seq nested) error = %v", err)
		}
	})

	t.Run("scalar delete is no-op", func(t *testing.T) {
		if err := deleteNode(&yaml.Node{Kind: yaml.ScalarNode}, []string{"a"}); err != nil {
			t.Fatalf("deleteNode(scalar) error = %v", err)
		}
	})
}

func TestApply_EmptyChangeset_NullYAML(t *testing.T) {
	doc := New()
	out, err := doc.Apply([]byte("null\n"), nil)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	got, err := doc.Get(out)
	if err != nil {
		t.Fatalf("Get(output) error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("Get(Apply(null)) = %#v, want empty map", got)
	}
}

func TestApply_WithChangeset_SkipsInvalidAndRootPaths(t *testing.T) {
	doc := New()
	out, err := doc.Apply([]byte("a: 1\n"), document.JSONPatchSet{
		{Op: document.PatchOpAdd, Path: "relative", Value: 1}, // invalid JSON pointer -> skipped
		{Op: document.PatchOpAdd, Path: "", Value: 1},         // root pointer -> keys empty -> ignored
		document.NewReplacePatch("/a", 2),
		document.NewRemovePatch(""), // root pointer -> keys empty -> ignored
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	parsed, err := doc.Get(out)
	if err != nil {
		t.Fatalf("Get(output) error = %v", err)
	}
	if parsed["a"] != 2 {
		t.Fatalf("parsed[a] = %#v, want 2", parsed["a"])
	}
}

func TestApply_NoChangeset_InvalidYAML(t *testing.T) {
	doc := New()
	_, err := doc.Apply([]byte("this: [\n"), nil)
	if err == nil {
		t.Fatal("Apply(invalid,nil) expected error, got nil")
	}
}
