package yaml

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
)

// TestNew tests creation of a new empty YAML document.
func TestNew(t *testing.T) {
	doc := New()
	if doc == nil {
		t.Fatal("New() returned nil")
	}

	if doc.Format() != document.FormatYAML {
		t.Errorf("Format() = %v, want %v", doc.Format(), document.FormatYAML)
	}

	// Empty document should marshal to empty mapping
	data, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Should be "{}\n" or similar empty mapping representation
	if len(data) == 0 {
		t.Error("Marshal() returned empty data for new document")
	}
}

// TestParse tests parsing of YAML documents.
func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid simple YAML",
			yaml: `
server:
  host: localhost
  port: 8080
`,
			wantErr: false,
		},
		{
			name: "valid YAML with comments",
			yaml: `
# Server configuration
server:
  host: localhost  # Default host
  port: 8080       # Default port
`,
			wantErr: false,
		},
		{
			name: "valid YAML with arrays",
			yaml: `
servers:
  - name: server1
    port: 8080
  - name: server2
    port: 8081
`,
			wantErr: false,
		},
		{
			name:    "empty YAML",
			yaml:    "",
			wantErr: false,
		},
		{
			name: "invalid YAML",
			yaml: `
server: [
  host: localhost
  port: 8080
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(tt.yaml))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && doc == nil {
				t.Error("Parse() returned nil document without error")
			}
		})
	}
}

// TestDocument_Get tests retrieving values from YAML documents.
func TestDocument_Get(t *testing.T) {
	yamlData := `
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
`

	doc, err := Parse([]byte(yamlData))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	tests := []struct {
		name   string
		path   string
		want   any
		wantOk bool
	}{
		{
			name:   "get string value",
			path:   "/server/host",
			want:   "localhost",
			wantOk: true,
		},
		{
			name:   "get int value",
			path:   "/server/port",
			want:   8080,
			wantOk: true,
		},
		{
			name:   "get bool value",
			path:   "/server/enabled",
			want:   true,
			wantOk: true,
		},
		{
			name:   "get float value",
			path:   "/server/ratio",
			want:   0.75,
			wantOk: true,
		},
		{
			name:   "get nested value",
			path:   "/database/connections/primary/host",
			want:   "db.example.com",
			wantOk: true,
		},
		{
			name:   "get array element",
			path:   "/servers/0/name",
			want:   "server1",
			wantOk: true,
		},
		{
			name:   "get second array element",
			path:   "/servers/1/port",
			want:   8081,
			wantOk: true,
		},
		{
			name:   "get entire array",
			path:   "/servers",
			want:   []any{map[string]any{"name": "server1", "port": 8080}, map[string]any{"name": "server2", "port": 8081}},
			wantOk: true,
		},
		{
			name:   "get entire object",
			path:   "/server",
			want:   map[string]any{"host": "localhost", "port": 8080, "enabled": true, "ratio": 0.75},
			wantOk: true,
		},
		{
			name:   "get empty map",
			path:   "/empty_map",
			want:   map[string]any{},
			wantOk: true,
		},
		{
			name:   "get null value",
			path:   "/null_value",
			want:   nil,
			wantOk: true,
		},
		{
			name:   "get non-existent path",
			path:   "/nonexistent",
			want:   nil,
			wantOk: false,
		},
		{
			name:   "get non-existent nested path",
			path:   "/server/nonexistent",
			want:   nil,
			wantOk: false,
		},
		{
			name:   "get out of bounds array index",
			path:   "/servers/10",
			want:   nil,
			wantOk: false,
		},
		{
			name:   "get negative array index",
			path:   "/servers/-1",
			want:   nil,
			wantOk: false,
		},
		{
			name:   "invalid path - no leading slash",
			path:   "server/port",
			want:   nil,
			wantOk: false,
		},
		{
			name:   "empty path returns root",
			path:   "",
			want:   nil, // Will be a map, we'll check separately
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := doc.Get(tt.path)
			if ok != tt.wantOk {
				t.Errorf("Get(%q) ok = %v, want %v", tt.path, ok, tt.wantOk)
				return
			}

			if !tt.wantOk {
				return
			}

			// Special handling for empty path (returns whole document)
			if tt.path == "" {
				if _, isMap := got.(map[string]any); !isMap {
					t.Errorf("Get(%q) = %v, want map[string]any", tt.path, got)
				}
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestDocument_Get_EscapedPaths tests JSON Pointer escaping.
func TestDocument_Get_EscapedPaths(t *testing.T) {
	yamlData := `
"feature/flags":
  enabled: true
"path~with~tildes":
  value: test
`

	doc, err := Parse([]byte(yamlData))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	tests := []struct {
		name   string
		path   string
		want   any
		wantOk bool
	}{
		{
			name:   "escaped slash in key",
			path:   "/feature~1flags/enabled",
			want:   true,
			wantOk: true,
		},
		{
			name:   "escaped tilde in key",
			path:   "/path~0with~0tildes/value",
			want:   "test",
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := doc.Get(tt.path)
			if ok != tt.wantOk {
				t.Errorf("Get(%q) ok = %v, want %v", tt.path, ok, tt.wantOk)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestDocument_Set tests setting values in YAML documents.
func TestDocument_Set(t *testing.T) {
	tests := []struct {
		name      string
		initial   string
		path      string
		value     any
		wantErr   bool
		checkPath string
		checkVal  any
	}{
		{
			name:      "set new string value",
			initial:   "{}",
			path:      "/server/host",
			value:     "localhost",
			wantErr:   false,
			checkPath: "/server/host",
			checkVal:  "localhost",
		},
		{
			name:      "set new int value",
			initial:   "{}",
			path:      "/server/port",
			value:     8080,
			wantErr:   false,
			checkPath: "/server/port",
			checkVal:  8080,
		},
		{
			name:      "set new bool value",
			initial:   "{}",
			path:      "/server/enabled",
			value:     true,
			wantErr:   false,
			checkPath: "/server/enabled",
			checkVal:  true,
		},
		{
			name:      "set new float value",
			initial:   "{}",
			path:      "/server/ratio",
			value:     0.75,
			wantErr:   false,
			checkPath: "/server/ratio",
			checkVal:  0.75,
		},
		{
			name:      "set null value",
			initial:   "{}",
			path:      "/server/value",
			value:     nil,
			wantErr:   false,
			checkPath: "/server/value",
			checkVal:  nil,
		},
		{
			name: "update existing value",
			initial: `
server:
  host: oldhost
  port: 8080
`,
			path:      "/server/host",
			value:     "newhost",
			wantErr:   false,
			checkPath: "/server/host",
			checkVal:  "newhost",
		},
		{
			name:      "set deeply nested value - creates intermediate nodes",
			initial:   "{}",
			path:      "/database/connections/primary/host",
			value:     "db.example.com",
			wantErr:   false,
			checkPath: "/database/connections/primary/host",
			checkVal:  "db.example.com",
		},
		{
			name:      "set array element at index 0 (append to empty array)",
			initial:   "servers: []",
			path:      "/servers/0",
			value:     "server1",
			wantErr:   false,
			checkPath: "/servers/0",
			checkVal:  "server1",
		},
		{
			name: "set array element at existing index",
			initial: `
servers:
  - server1
  - server2
`,
			path:      "/servers/1",
			value:     "newserver",
			wantErr:   false,
			checkPath: "/servers/1",
			checkVal:  "newserver",
		},
		{
			name:      "set nested array element - creates intermediate nodes",
			initial:   "{}",
			path:      "/servers/0/name",
			value:     "server1",
			wantErr:   false,
			checkPath: "/servers/0/name",
			checkVal:  "server1",
		},
		{
			name:      "set map value",
			initial:   "{}",
			path:      "/server",
			value:     map[string]any{"host": "localhost", "port": 8080},
			wantErr:   false,
			checkPath: "/server/host",
			checkVal:  "localhost",
		},
		{
			name:      "set array value",
			initial:   "{}",
			path:      "/servers",
			value:     []any{"server1", "server2"},
			wantErr:   false,
			checkPath: "/servers/0",
			checkVal:  "server1",
		},
		{
			name:      "error - cannot set root",
			initial:   "{}",
			path:      "",
			value:     "value",
			wantErr:   true,
			checkPath: "",
			checkVal:  nil,
		},
		{
			name:      "error - invalid path (no leading slash)",
			initial:   "{}",
			path:      "server/port",
			value:     8080,
			wantErr:   true,
			checkPath: "",
			checkVal:  nil,
		},
		{
			name: "error - array index out of range",
			initial: `
servers:
  - server1
`,
			path:      "/servers/10",
			value:     "server2",
			wantErr:   true,
			checkPath: "",
			checkVal:  nil,
		},
		{
			name: "error - negative array index",
			initial: `
servers:
  - server1
`,
			path:      "/servers/-1",
			value:     "server2",
			wantErr:   true,
			checkPath: "",
			checkVal:  nil,
		},
		{
			name: "error - non-numeric array index",
			initial: `
servers:
  - server1
`,
			path:      "/servers/abc",
			value:     "server2",
			wantErr:   true,
			checkPath: "",
			checkVal:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(tt.initial))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			err = doc.Set(tt.path, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set(%q, %v) error = %v, wantErr %v", tt.path, tt.value, err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Verify the value was set
			got, ok := doc.Get(tt.checkPath)
			if !ok {
				t.Errorf("Get(%q) after Set() returned false", tt.checkPath)
				return
			}

			if !reflect.DeepEqual(got, tt.checkVal) {
				t.Errorf("Get(%q) after Set() = %v, want %v", tt.checkPath, got, tt.checkVal)
			}
		})
	}
}

// TestDocument_Set_EscapedPaths tests setting values with escaped paths.
func TestDocument_Set_EscapedPaths(t *testing.T) {
	doc := New()

	// Set a key with "/" in it (escaped as ~1)
	err := doc.Set("/feature~1flags/enabled", true)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify using the same escaped path
	got, ok := doc.Get("/feature~1flags/enabled")
	if !ok {
		t.Error("Get() returned false after Set()")
	}
	if got != true {
		t.Errorf("Get() = %v, want true", got)
	}

	// Set a key with "~" in it (escaped as ~0)
	err = doc.Set("/path~0with~0tildes/value", "test")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, ok = doc.Get("/path~0with~0tildes/value")
	if !ok {
		t.Error("Get() returned false after Set()")
	}
	if got != "test" {
		t.Errorf("Get() = %v, want test", got)
	}
}

// TestDocument_Delete tests deleting values from YAML documents.
func TestDocument_Delete(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		deletePath string
		wantErr    bool
		checkPath  string
		wantExist  bool
	}{
		{
			name: "delete existing value",
			initial: `
server:
  host: localhost
  port: 8080
`,
			deletePath: "/server/host",
			wantErr:    false,
			checkPath:  "/server/host",
			wantExist:  false,
		},
		{
			name: "delete nested value",
			initial: `
database:
  connections:
    primary:
      host: db.example.com
      port: 5432
`,
			deletePath: "/database/connections/primary/host",
			wantErr:    false,
			checkPath:  "/database/connections/primary/host",
			wantExist:  false,
		},
		{
			name: "delete array element",
			initial: `
servers:
  - server1
  - server2
  - server3
`,
			deletePath: "/servers/1",
			wantErr:    false,
			checkPath:  "/servers/1",
			wantExist:  true, // server3 moves to index 1
		},
		{
			name: "delete entire object",
			initial: `
server:
  host: localhost
  port: 8080
database:
  host: db.example.com
`,
			deletePath: "/server",
			wantErr:    false,
			checkPath:  "/server",
			wantExist:  false,
		},
		{
			name: "delete non-existent path (idempotent)",
			initial: `
server:
  host: localhost
`,
			deletePath: "/nonexistent",
			wantErr:    false,
			checkPath:  "/server/host",
			wantExist:  true, // Other values remain
		},
		{
			name: "delete non-existent nested path (idempotent)",
			initial: `
server:
  host: localhost
`,
			deletePath: "/server/nonexistent",
			wantErr:    false,
			checkPath:  "/server/host",
			wantExist:  true,
		},
		{
			name:       "error - cannot delete root",
			initial:    "{}",
			deletePath: "",
			wantErr:    true,
			checkPath:  "",
			wantExist:  false,
		},
		{
			name:       "error - invalid path (no leading slash)",
			initial:    "{}",
			deletePath: "server/port",
			wantErr:    true,
			checkPath:  "",
			wantExist:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(tt.initial))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			err = doc.Delete(tt.deletePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Delete(%q) error = %v, wantErr %v", tt.deletePath, err, tt.wantErr)
				return
			}

			if tt.wantErr || tt.checkPath == "" {
				return
			}

			// Verify the value was deleted (or exists)
			_, ok := doc.Get(tt.checkPath)
			if ok != tt.wantExist {
				t.Errorf("Get(%q) after Delete() = %v, want %v", tt.checkPath, ok, tt.wantExist)
			}
		})
	}
}

// TestDocument_Marshal tests marshaling YAML documents.
func TestDocument_Marshal(t *testing.T) {
	tests := []struct {
		name      string
		initial   string
		wantErr   bool
		wantEmpty bool
	}{
		{
			name: "marshal simple document",
			initial: `
server:
  host: localhost
  port: 8080
`,
			wantErr:   false,
			wantEmpty: false,
		},
		{
			name: "marshal document with arrays",
			initial: `
servers:
  - name: server1
    port: 8080
  - name: server2
    port: 8081
`,
			wantErr:   false,
			wantEmpty: false,
		},
		{
			name:      "marshal empty document",
			initial:   "{}",
			wantErr:   false,
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(tt.initial))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			data, err := doc.Marshal()
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantEmpty && len(data) > 0 {
				t.Errorf("Marshal() = %q, want empty", string(data))
			}

			if !tt.wantEmpty && len(data) == 0 {
				t.Error("Marshal() returned empty data")
			}

			// Verify round-trip: parse the marshaled data
			if !tt.wantErr && len(data) > 0 {
				_, err := Parse(data)
				if err != nil {
					t.Errorf("Parse(Marshal()) error = %v", err)
				}
			}
		})
	}
}

// TestDocument_CommentPreservation tests that comments are preserved.
func TestDocument_CommentPreservation(t *testing.T) {
	yamlData := `
# Server configuration
server:
  host: localhost  # Default host
  port: 8080       # Default port
`

	doc, err := Parse([]byte(yamlData))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Modify a value
	err = doc.Set("/server/port", 9000)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Marshal and check comments are preserved
	data, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	output := string(data)

	// Check that comments are present
	if !strings.Contains(output, "# Server configuration") {
		t.Error("Marshal() did not preserve heading comment")
	}

	if !strings.Contains(output, "# Default host") {
		t.Error("Marshal() did not preserve inline comment for host")
	}

	if !strings.Contains(output, "# Default port") {
		t.Error("Marshal() did not preserve inline comment for port")
	}

	// Check that the value was updated
	if !strings.Contains(output, "9000") {
		t.Error("Marshal() did not include updated port value")
	}
}

// TestDocument_ComplexScenario tests a complex real-world scenario.
func TestDocument_ComplexScenario(t *testing.T) {
	// Start with a configuration file
	initial := `
# Application configuration
app:
  name: myapp
  version: 1.0.0

# Server settings
server:
  host: localhost
  port: 8080
  features:
    - ssl
    - compression

# Database settings
database:
  primary:
    host: db.example.com
    port: 5432
  replicas: []
`

	doc, err := Parse([]byte(initial))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Update existing values
	if err := doc.Set("/server/port", 9000); err != nil {
		t.Errorf("Set(/server/port) error = %v", err)
	}

	// Add new feature to array
	if err := doc.Set("/server/features/2", "caching"); err != nil {
		t.Errorf("Set(/server/features/2) error = %v", err)
	}

	// Add replica
	if err := doc.Set("/database/replicas/0", map[string]any{
		"host": "replica1.example.com",
		"port": 5432,
	}); err != nil {
		t.Errorf("Set(/database/replicas/0) error = %v", err)
	}

	// Add new top-level section
	if err := doc.Set("/logging/level", "info"); err != nil {
		t.Errorf("Set(/logging/level) error = %v", err)
	}

	// Delete a value
	if err := doc.Delete("/app/version"); err != nil {
		t.Errorf("Delete(/app/version) error = %v", err)
	}

	// Marshal and verify
	data, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	output := string(data)

	// Verify comments are preserved
	if !strings.Contains(output, "# Application configuration") {
		t.Error("Comment not preserved: # Application configuration")
	}
	if !strings.Contains(output, "# Server settings") {
		t.Error("Comment not preserved: # Server settings")
	}
	if !strings.Contains(output, "# Database settings") {
		t.Error("Comment not preserved: # Database settings")
	}

	// Verify values are correct
	if port, ok := doc.Get("/server/port"); !ok || port != 9000 {
		t.Errorf("Get(/server/port) = %v, want 9000", port)
	}

	if feature, ok := doc.Get("/server/features/2"); !ok || feature != "caching" {
		t.Errorf("Get(/server/features/2) = %v, want caching", feature)
	}

	if replica, ok := doc.Get("/database/replicas/0"); !ok {
		t.Error("Get(/database/replicas/0) failed")
	} else if m, ok := replica.(map[string]any); ok {
		if m["host"] != "replica1.example.com" {
			t.Errorf("replica host = %v, want replica1.example.com", m["host"])
		}
	}

	if level, ok := doc.Get("/logging/level"); !ok || level != "info" {
		t.Errorf("Get(/logging/level) = %v, want info", level)
	}

	if _, ok := doc.Get("/app/version"); ok {
		t.Error("Get(/app/version) should not exist after Delete()")
	}
}

// TestDocument_RoundTrip tests round-trip parsing and marshaling.
func TestDocument_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "simple document",
			yaml: `
server:
  host: localhost
  port: 8080
`,
		},
		{
			name: "document with arrays",
			yaml: `
servers:
  - name: server1
    port: 8080
  - name: server2
    port: 8081
`,
		},
		{
			name: "document with mixed types",
			yaml: `
string: value
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
			doc1, err := Parse([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			// Marshal
			data, err := doc1.Marshal()
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// Parse again
			doc2, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse(Marshal()) error = %v", err)
			}

			// Compare root values
			val1, ok1 := doc1.Get("")
			val2, ok2 := doc2.Get("")

			if ok1 != ok2 {
				t.Errorf("Round-trip Get() ok mismatch: %v vs %v", ok1, ok2)
			}

			if !reflect.DeepEqual(val1, val2) {
				t.Errorf("Round-trip value mismatch:\noriginal: %v\nround-trip: %v", val1, val2)
			}
		})
	}
}

// TestDocument_ErrorTypes tests that proper error types are returned.
func TestDocument_ErrorTypes(t *testing.T) {
	doc := New()

	// Set a value to test on
	doc.Set("/server/port", 8080)

	tests := []struct {
		name      string
		operation func() error
		wantType  any
	}{
		{
			name: "Set with invalid path returns InvalidPathError",
			operation: func() error {
				return doc.Set("", "value")
			},
			wantType: &document.InvalidPathError{},
		},
		{
			name: "Delete with invalid path returns InvalidPathError",
			operation: func() error {
				return doc.Delete("")
			},
			wantType: &document.InvalidPathError{},
		},
		{
			name: "Set with invalid array index returns InvalidPathError",
			operation: func() error {
				doc.Set("/servers", []any{"server1"})
				return doc.Set("/servers/abc", "server2")
			},
			wantType: &document.InvalidPathError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.operation()
			if err == nil {
				t.Error("expected error, got nil")
				return
			}

			if !errors.As(err, &tt.wantType) {
				t.Errorf("error type = %T, want %T", err, tt.wantType)
			}
		})
	}
}

// TestDocument_ConcurrentRead tests concurrent reads.
func TestDocument_ConcurrentRead(t *testing.T) {
	yamlData := `
server:
  host: localhost
  port: 8080
`

	doc, err := Parse([]byte(yamlData))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Concurrent reads should be safe
	const numReaders = 10
	done := make(chan bool)

	for i := 0; i < numReaders; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				if _, ok := doc.Get("/server/port"); !ok {
					t.Error("Get() failed during concurrent read")
				}
			}
			done <- true
		}()
	}

	for i := 0; i < numReaders; i++ {
		<-done
	}
}

// TestDocument_Interface tests that Document implements document.Document interface.
func TestDocument_Interface(t *testing.T) {
	var _ document.Document = (*Document)(nil)
	var _ document.Document = New()
}

// TestDocument_DistinguishNullEmptyZeroMissing tests that the document correctly
// distinguishes between:
//   - Missing key (key does not exist)
//   - Null value (explicit null)
//   - Zero values (0, false, "")
func TestDocument_DistinguishNullEmptyZeroMissing(t *testing.T) {
	yamlData := `
# Test distinguishing between null, empty, zero, and missing values
explicit_null: null
explicit_null_tilde: ~
empty_string: ""
zero_int: 0
zero_float: 0.0
false_bool: false
# missing_key is intentionally not defined
`

	doc, err := Parse([]byte(yamlData))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	tests := []struct {
		name     string
		path     string
		wantOk   bool
		wantVal  any
		wantType string // for debugging
	}{
		{
			name:     "missing key returns (nil, false)",
			path:     "/missing_key",
			wantOk:   false,
			wantVal:  nil,
			wantType: "missing",
		},
		{
			name:     "explicit null returns (nil, true)",
			path:     "/explicit_null",
			wantOk:   true,
			wantVal:  nil,
			wantType: "null",
		},
		{
			name:     "explicit null with tilde returns (nil, true)",
			path:     "/explicit_null_tilde",
			wantOk:   true,
			wantVal:  nil,
			wantType: "null",
		},
		{
			name:     "empty string returns (\"\", true)",
			path:     "/empty_string",
			wantOk:   true,
			wantVal:  "",
			wantType: "empty string",
		},
		{
			name:     "zero int returns (0, true)",
			path:     "/zero_int",
			wantOk:   true,
			wantVal:  0,
			wantType: "zero int",
		},
		{
			name:     "zero float returns (0.0, true)",
			path:     "/zero_float",
			wantOk:   true,
			wantVal:  0.0,
			wantType: "zero float",
		},
		{
			name:     "false bool returns (false, true)",
			path:     "/false_bool",
			wantOk:   true,
			wantVal:  false,
			wantType: "false bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := doc.Get(tt.path)

			if ok != tt.wantOk {
				t.Errorf("Get(%q) ok = %v, want %v (type: %s)", tt.path, ok, tt.wantOk, tt.wantType)
				return
			}

			if !tt.wantOk {
				// For missing keys, we only check that ok is false
				return
			}

			// For existing keys, check the value
			if !reflect.DeepEqual(got, tt.wantVal) {
				t.Errorf("Get(%q) = %v (%T), want %v (%T) (type: %s)",
					tt.path, got, got, tt.wantVal, tt.wantVal, tt.wantType)
			}
		})
	}
}

// TestDocument_SetAndGetEmptyString tests round-trip for empty string values.
func TestDocument_SetAndGetEmptyString(t *testing.T) {
	doc := New()

	// Set an empty string
	err := doc.Set("/config/value", "")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get it back
	got, ok := doc.Get("/config/value")
	if !ok {
		t.Fatal("Get() returned false for set empty string")
	}

	if got != "" {
		t.Errorf("Get() = %v (%T), want \"\" (string)", got, got)
	}

	// Marshal and parse again
	data, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	doc2, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	got2, ok2 := doc2.Get("/config/value")
	if !ok2 {
		t.Fatal("Get() returned false after round-trip")
	}

	if got2 != "" {
		t.Errorf("Get() after round-trip = %v (%T), want \"\" (string)", got2, got2)
	}
}

// TestDocument_SetAndGetNull tests round-trip for null values.
func TestDocument_SetAndGetNull(t *testing.T) {
	doc := New()

	// Set a null value
	err := doc.Set("/config/value", nil)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get it back
	got, ok := doc.Get("/config/value")
	if !ok {
		t.Fatal("Get() returned false for set null value")
	}

	if got != nil {
		t.Errorf("Get() = %v (%T), want nil", got, got)
	}

	// Marshal and parse again
	data, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	doc2, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	got2, ok2 := doc2.Get("/config/value")
	if !ok2 {
		t.Fatal("Get() returned false after round-trip")
	}

	if got2 != nil {
		t.Errorf("Get() after round-trip = %v (%T), want nil", got2, got2)
	}
}

// TestDocument_Compliance runs the standard Document compliance tests.
func TestDocument_Compliance(t *testing.T) {
	jktest.NewDocumentTester(t, NewParser()).TestAll()
}

// TestDocument_Anchors tests YAML anchor and alias functionality.
func TestDocument_Anchors(t *testing.T) {
	t.Run("get value through alias", func(t *testing.T) {
		yamlData := `
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
`
		doc, err := Parse([]byte(yamlData))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		// Get value from anchor directly
		host, ok := doc.Get("/defaults/host")
		if !ok {
			t.Error("Get(/defaults/host) returned false")
		}
		if host != "localhost" {
			t.Errorf("Get(/defaults/host) = %v, want localhost", host)
		}

		// Note: YAML merge keys (<<) are handled by the YAML parser during unmarshaling,
		// so we can verify that merged values are accessible
		devDB, ok := doc.Get("/development/database")
		if !ok {
			t.Error("Get(/development/database) returned false")
		}
		if devDB != "dev_db" {
			t.Errorf("Get(/development/database) = %v, want dev_db", devDB)
		}
	})

	t.Run("get value from alias reference", func(t *testing.T) {
		yamlData := `
base: &base
  name: baseValue
  nested:
    key: nestedValue

ref: *base
`
		doc, err := Parse([]byte(yamlData))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		// Get value through alias
		name, ok := doc.Get("/ref/name")
		if !ok {
			t.Error("Get(/ref/name) returned false")
		}
		if name != "baseValue" {
			t.Errorf("Get(/ref/name) = %v, want baseValue", name)
		}

		// Get nested value through alias
		nested, ok := doc.Get("/ref/nested/key")
		if !ok {
			t.Error("Get(/ref/nested/key) returned false")
		}
		if nested != "nestedValue" {
			t.Errorf("Get(/ref/nested/key) = %v, want nestedValue", nested)
		}
	})

	t.Run("get alias in array", func(t *testing.T) {
		yamlData := `
item: &item
  id: 1
  name: test

items:
  - *item
  - id: 2
    name: other
`
		doc, err := Parse([]byte(yamlData))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		// Get value through alias in array
		id, ok := doc.Get("/items/0/id")
		if !ok {
			t.Error("Get(/items/0/id) returned false")
		}
		if id != 1 {
			t.Errorf("Get(/items/0/id) = %v, want 1", id)
		}

		name, ok := doc.Get("/items/0/name")
		if !ok {
			t.Error("Get(/items/0/name) returned false")
		}
		if name != "test" {
			t.Errorf("Get(/items/0/name) = %v, want test", name)
		}
	})

	t.Run("set value through alias modifies anchor target", func(t *testing.T) {
		yamlData := `
base: &base
  value: original

ref: *base
`
		doc, err := Parse([]byte(yamlData))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		// Set value through alias
		err = doc.Set("/ref/value", "modified")
		if err != nil {
			t.Fatalf("Set(/ref/value) error = %v", err)
		}

		// Verify the value through alias
		val, ok := doc.Get("/ref/value")
		if !ok {
			t.Error("Get(/ref/value) returned false")
		}
		if val != "modified" {
			t.Errorf("Get(/ref/value) = %v, want modified", val)
		}

		// Verify the anchor target was also modified
		baseVal, ok := doc.Get("/base/value")
		if !ok {
			t.Error("Get(/base/value) returned false")
		}
		if baseVal != "modified" {
			t.Errorf("Get(/base/value) = %v, want modified (anchor target should be modified)", baseVal)
		}
	})

	t.Run("delete key through alias", func(t *testing.T) {
		yamlData := `
base: &base
  key1: value1
  key2: value2

ref: *base
`
		doc, err := Parse([]byte(yamlData))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		// Delete key through alias (modifies anchor target)
		err = doc.Delete("/ref/key1")
		if err != nil {
			t.Fatalf("Delete(/ref/key1) error = %v", err)
		}

		// Verify the key was deleted through alias
		_, ok := doc.Get("/ref/key1")
		if ok {
			t.Error("Get(/ref/key1) should return false after delete")
		}

		// Verify the anchor target was also modified
		_, ok = doc.Get("/base/key1")
		if ok {
			t.Error("Get(/base/key1) should return false (anchor target should be modified)")
		}

		// Other keys should still exist
		val, ok := doc.Get("/ref/key2")
		if !ok {
			t.Error("Get(/ref/key2) should return true")
		}
		if val != "value2" {
			t.Errorf("Get(/ref/key2) = %v, want value2", val)
		}
	})

	t.Run("scalar alias", func(t *testing.T) {
		yamlData := `
default_port: &port 8080
server:
  port: *port
`
		doc, err := Parse([]byte(yamlData))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		// Get scalar value through alias
		port, ok := doc.Get("/server/port")
		if !ok {
			t.Error("Get(/server/port) returned false")
		}
		if port != 8080 {
			t.Errorf("Get(/server/port) = %v, want 8080", port)
		}
	})

	t.Run("nodeToValue with alias in mapping", func(t *testing.T) {
		yamlData := `
shared: &shared
  x: 1
  y: 2

data:
  point: *shared
`
		doc, err := Parse([]byte(yamlData))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		// Get entire aliased object
		point, ok := doc.Get("/data/point")
		if !ok {
			t.Error("Get(/data/point) returned false")
		}

		m, ok := point.(map[string]any)
		if !ok {
			t.Fatalf("Get(/data/point) = %T, want map[string]any", point)
		}

		if m["x"] != 1 {
			t.Errorf("point.x = %v, want 1", m["x"])
		}
		if m["y"] != 2 {
			t.Errorf("point.y = %v, want 2", m["y"])
		}
	})
}
