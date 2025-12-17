package env

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestBuildSchemaMapping_Simple(t *testing.T) {
	type Config struct {
		Port int    `json:"port" jubako:"env:SERVER_PORT"`
		Host string `json:"host" jubako:"env:SERVER_HOST"`
	}

	schema := BuildSchemaMapping[Config]()

	if len(schema.Mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(schema.Mappings))
	}

	// Check SERVER_PORT mapping
	portMapping, ok := schema.Mappings["SERVER_PORT"]
	if !ok {
		t.Fatal("SERVER_PORT mapping not found")
	}
	if portMapping.JSONPath != "/port" {
		t.Errorf("SERVER_PORT path = %q, want /port", portMapping.JSONPath)
	}
	if portMapping.FieldType.Kind() != reflect.Int {
		t.Errorf("SERVER_PORT type = %v, want int", portMapping.FieldType.Kind())
	}

	// Check SERVER_HOST mapping
	hostMapping, ok := schema.Mappings["SERVER_HOST"]
	if !ok {
		t.Fatal("SERVER_HOST mapping not found")
	}
	if hostMapping.JSONPath != "/host" {
		t.Errorf("SERVER_HOST path = %q, want /host", hostMapping.JSONPath)
	}
	if hostMapping.FieldType.Kind() != reflect.String {
		t.Errorf("SERVER_HOST type = %v, want string", hostMapping.FieldType.Kind())
	}
}

func TestBuildSchemaMapping_Nested(t *testing.T) {
	type Database struct {
		Host string `json:"host" jubako:"env:DB_HOST"`
		Port int    `json:"port" jubako:"env:DB_PORT"`
	}

	type Config struct {
		Database Database `json:"database"`
		AppName  string   `json:"app_name" jubako:"env:APP_NAME"`
	}

	schema := BuildSchemaMapping[Config]()

	if len(schema.Mappings) != 3 {
		t.Fatalf("expected 3 mappings, got %d", len(schema.Mappings))
	}

	// Check nested mappings
	dbHostMapping, ok := schema.Mappings["DB_HOST"]
	if !ok {
		t.Fatal("DB_HOST mapping not found")
	}
	if dbHostMapping.JSONPath != "/database/host" {
		t.Errorf("DB_HOST path = %q, want /database/host", dbHostMapping.JSONPath)
	}

	dbPortMapping, ok := schema.Mappings["DB_PORT"]
	if !ok {
		t.Fatal("DB_PORT mapping not found")
	}
	if dbPortMapping.JSONPath != "/database/port" {
		t.Errorf("DB_PORT path = %q, want /database/port", dbPortMapping.JSONPath)
	}
}

func TestBuildSchemaMapping_CustomPath(t *testing.T) {
	type Config struct {
		// Custom path overrides the field path
		Port int `json:"port" jubako:"/server/listen_port,env:PORT"`
	}

	schema := BuildSchemaMapping[Config]()

	if len(schema.Mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(schema.Mappings))
	}

	portMapping, ok := schema.Mappings["PORT"]
	if !ok {
		t.Fatal("PORT mapping not found")
	}
	if portMapping.JSONPath != "/server/listen_port" {
		t.Errorf("PORT path = %q, want /server/listen_port", portMapping.JSONPath)
	}
}

func TestBuildSchemaMapping_NoEnvTag(t *testing.T) {
	type Config struct {
		Port int    `json:"port"`                         // No jubako tag
		Host string `json:"host" jubako:"/custom/path"`   // jubako tag but no env:
		Name string `json:"name" jubako:"env:APP_NAME"`   // Has env:
	}

	schema := BuildSchemaMapping[Config]()

	// Only APP_NAME should be in the schema
	if len(schema.Mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(schema.Mappings))
	}

	_, ok := schema.Mappings["APP_NAME"]
	if !ok {
		t.Fatal("APP_NAME mapping not found")
	}
}

func TestBuildSchemaMapping_VariousTypes(t *testing.T) {
	type Config struct {
		IntVal      int           `json:"int_val" jubako:"env:INT_VAL"`
		Int64Val    int64         `json:"int64_val" jubako:"env:INT64_VAL"`
		UintVal     uint          `json:"uint_val" jubako:"env:UINT_VAL"`
		FloatVal    float64       `json:"float_val" jubako:"env:FLOAT_VAL"`
		BoolVal     bool          `json:"bool_val" jubako:"env:BOOL_VAL"`
		StringSlice []string      `json:"string_slice" jubako:"env:STRING_SLICE"`
		Duration    time.Duration `json:"duration" jubako:"env:DURATION"`
	}

	schema := BuildSchemaMapping[Config]()

	if len(schema.Mappings) != 7 {
		t.Fatalf("expected 7 mappings, got %d", len(schema.Mappings))
	}

	// Verify types
	tests := []struct {
		envVar   string
		wantKind reflect.Kind
	}{
		{"INT_VAL", reflect.Int},
		{"INT64_VAL", reflect.Int64},
		{"UINT_VAL", reflect.Uint},
		{"FLOAT_VAL", reflect.Float64},
		{"BOOL_VAL", reflect.Bool},
		{"STRING_SLICE", reflect.Slice},
		{"DURATION", reflect.Int64}, // time.Duration is int64
	}

	for _, tt := range tests {
		mapping, ok := schema.Mappings[tt.envVar]
		if !ok {
			t.Errorf("%s mapping not found", tt.envVar)
			continue
		}
		if mapping.FieldType.Kind() != tt.wantKind {
			t.Errorf("%s type = %v, want %v", tt.envVar, mapping.FieldType.Kind(), tt.wantKind)
		}
	}
}

func TestConvertStringToType(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		targetType reflect.Type
		want       any
		wantErr    bool
	}{
		{"string", "hello", reflect.TypeOf(""), "hello", false},
		{"int", "42", reflect.TypeOf(int(0)), 42, false},
		{"int negative", "-10", reflect.TypeOf(int(0)), -10, false},
		{"int invalid", "abc", reflect.TypeOf(int(0)), 0, true},
		{"int64", "9223372036854775807", reflect.TypeOf(int64(0)), int64(9223372036854775807), false},
		{"uint", "42", reflect.TypeOf(uint(0)), uint(42), false},
		{"uint64", "18446744073709551615", reflect.TypeOf(uint64(0)), uint64(18446744073709551615), false},
		{"float64", "3.14", reflect.TypeOf(float64(0)), 3.14, false},
		{"float32", "3.14", reflect.TypeOf(float32(0)), float32(3.14), false},
		{"bool true", "true", reflect.TypeOf(false), true, false},
		{"bool false", "false", reflect.TypeOf(false), false, false},
		{"bool 1", "1", reflect.TypeOf(false), true, false},
		{"bool 0", "0", reflect.TypeOf(false), false, false},
		{"bool invalid", "invalid", reflect.TypeOf(false), false, true},
		{"string slice", "a,b,c", reflect.TypeOf([]string{}), []string{"a", "b", "c"}, false},
		{"string slice empty", "", reflect.TypeOf([]string{}), []string{}, false},
		{"duration", "1h30m", reflect.TypeOf(time.Duration(0)), time.Hour + 30*time.Minute, false},
		{"duration invalid", "invalid", reflect.TypeOf(time.Duration(0)), time.Duration(0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertStringToType(tt.value, tt.targetType)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertStringToType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertStringToType() = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestSchemaMapping_CreateTransformFunc(t *testing.T) {
	type Config struct {
		Port int    `json:"port" jubako:"env:SERVER_PORT"`
		Host string `json:"host" jubako:"env:SERVER_HOST"`
	}

	schema := BuildSchemaMapping[Config]()
	transform := schema.CreateTransformFunc()

	// Test mapped env var with int conversion
	path, value := transform("SERVER_PORT", "8080")
	if path != "/port" {
		t.Errorf("SERVER_PORT path = %q, want /port", path)
	}
	if value != 8080 {
		t.Errorf("SERVER_PORT value = %v, want 8080", value)
	}

	// Test mapped env var with string
	path, value = transform("SERVER_HOST", "localhost")
	if path != "/host" {
		t.Errorf("SERVER_HOST path = %q, want /host", path)
	}
	if value != "localhost" {
		t.Errorf("SERVER_HOST value = %v, want localhost", value)
	}

	// Test unmapped env var
	path, value = transform("UNKNOWN_VAR", "value")
	if path != "" {
		t.Errorf("UNKNOWN_VAR path = %q, want empty", path)
	}

	// Test invalid type conversion (returns empty path)
	path, value = transform("SERVER_PORT", "invalid")
	if path != "" {
		t.Errorf("invalid SERVER_PORT path = %q, want empty (skipped)", path)
	}
}

func TestWithSchemaMapping_Integration(t *testing.T) {
	type Config struct {
		Port     int           `json:"port" jubako:"env:PORT"`
		Host     string        `json:"host" jubako:"env:HOST"`
		Debug    bool          `json:"debug" jubako:"env:DEBUG"`
		Timeout  time.Duration `json:"timeout" jubako:"env:TIMEOUT"`
		Features []string      `json:"features" jubako:"env:FEATURES"`
	}

	// Create layer with schema mapping
	layer := New("test", "APP_",
		WithSchemaMapping[Config](),
		WithEnvironFunc(func() []string {
			return []string{
				"APP_PORT=8080",
				"APP_HOST=localhost",
				"APP_DEBUG=true",
				"APP_TIMEOUT=30s",
				"APP_FEATURES=feature1,feature2,feature3",
				"APP_UNKNOWN=ignored", // Should be skipped
			}
		}),
	)

	// Load the layer
	data, err := layer.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify values
	if port, ok := data["port"].(int); !ok || port != 8080 {
		t.Errorf("port = %v, want 8080", data["port"])
	}
	if host, ok := data["host"].(string); !ok || host != "localhost" {
		t.Errorf("host = %v, want localhost", data["host"])
	}
	if debug, ok := data["debug"].(bool); !ok || !debug {
		t.Errorf("debug = %v, want true", data["debug"])
	}
	if timeout, ok := data["timeout"].(time.Duration); !ok || timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", data["timeout"])
	}
	if features, ok := data["features"].([]string); !ok || len(features) != 3 {
		t.Errorf("features = %v, want [feature1 feature2 feature3]", data["features"])
	}

	// UNKNOWN should not be in data (no mapping)
	if _, ok := data["unknown"]; ok {
		t.Error("UNKNOWN should not be in data")
	}
}

func TestWithSchemaMapping_NestedStruct(t *testing.T) {
	type Server struct {
		Port int    `json:"port" jubako:"env:SERVER_PORT"`
		Host string `json:"host" jubako:"env:SERVER_HOST"`
	}

	type Config struct {
		Server Server `json:"server"`
	}

	layer := New("test", "APP_",
		WithSchemaMapping[Config](),
		WithEnvironFunc(func() []string {
			return []string{
				"APP_SERVER_PORT=9000",
				"APP_SERVER_HOST=0.0.0.0",
			}
		}),
	)

	data, err := layer.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check nested structure
	server, ok := data["server"].(map[string]any)
	if !ok {
		t.Fatalf("server = %v (%T), want map[string]any", data["server"], data["server"])
	}

	if port, ok := server["port"].(int); !ok || port != 9000 {
		t.Errorf("server.port = %v, want 9000", server["port"])
	}
	if host, ok := server["host"].(string); !ok || host != "0.0.0.0" {
		t.Errorf("server.host = %v, want 0.0.0.0", server["host"])
	}
}

// Note: parseEnvDirective was removed and its functionality is now handled by
// internal/tag.Parse. The env directive parsing is tested through
// TestBuildSchemaMapping_* tests and store_test.go's TestParseJubakoTag tests.
