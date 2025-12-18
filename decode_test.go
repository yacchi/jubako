package jubako

import (
	"context"
	"testing"

	"github.com/yacchi/jubako/layer/mapdata"
)

func TestDecodeWithTags_PathMapping(t *testing.T) {
	ctx := context.Background()

	// Config struct with jubako tags for path remapping
	type ServerConfig struct {
		Host             string `json:"host" jubako:"/server/host"`
		Port             int    `json:"port" jubako:"/server/port"`
		HTTPReadTimeout  int    `json:"http_read_timeout" jubako:"/server/http/read_timeout"`
		HTTPWriteTimeout int    `json:"http_write_timeout" jubako:"/server/http/write_timeout"`
		CookieSecret     string `json:"cookie_secret" jubako:"/server/cookie/secret"`
		CookieMaxAge     int    `json:"cookie_max_age" jubako:"/server/cookie/max_age"`
		RateLimitEnabled bool   `json:"rate_limit_enabled" jubako:"/server/rate_limit/enabled"`
		RateLimitRPM     int    `json:"rate_limit_rpm" jubako:"/server/rate_limit/requests_per_minute"`
	}

	// Nested YAML-like structure
	data := map[string]any{
		"server": map[string]any{
			"host": "0.0.0.0",
			"port": 8080,
			"http": map[string]any{
				"read_timeout":  10,
				"write_timeout": 30,
			},
			"cookie": map[string]any{
				"secret":  "my-secret",
				"max_age": 300,
			},
			"rate_limit": map[string]any{
				"enabled":             true,
				"requests_per_minute": 60,
			},
		},
	}

	store := New[ServerConfig]()
	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	// Verify all fields are correctly mapped
	tests := []struct {
		name     string
		got      any
		expected any
	}{
		{"Host", config.Host, "0.0.0.0"},
		{"Port", config.Port, 8080},
		{"HTTPReadTimeout", config.HTTPReadTimeout, 10},
		{"HTTPWriteTimeout", config.HTTPWriteTimeout, 30},
		{"CookieSecret", config.CookieSecret, "my-secret"},
		{"CookieMaxAge", config.CookieMaxAge, 300},
		{"RateLimitEnabled", config.RateLimitEnabled, true},
		{"RateLimitRPM", config.RateLimitRPM, 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestDecodeWithTags_MixedFields(t *testing.T) {
	ctx := context.Background()

	// Mix of jubako-tagged and regular fields
	type AppConfig struct {
		// Fields with jubako tags (path remapping)
		AppName    string `json:"app_name" jubako:"/application/name"`
		AppVersion string `json:"app_version" jubako:"/application/version"`
		// Fields without jubako tags (regular JSON mapping)
		Debug    bool   `json:"debug"`
		LogLevel string `json:"log_level"`
	}

	data := map[string]any{
		"application": map[string]any{
			"name":    "MyApp",
			"version": "1.0.0",
		},
		"debug":     true,
		"log_level": "info",
	}

	store := New[AppConfig]()
	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	if config.AppName != "MyApp" {
		t.Errorf("AppName = %v, want MyApp", config.AppName)
	}
	if config.AppVersion != "1.0.0" {
		t.Errorf("AppVersion = %v, want 1.0.0", config.AppVersion)
	}
	if config.Debug != true {
		t.Errorf("Debug = %v, want true", config.Debug)
	}
	if config.LogLevel != "info" {
		t.Errorf("LogLevel = %v, want info", config.LogLevel)
	}
}

func TestDecodeWithTags_NoTags(t *testing.T) {
	ctx := context.Background()

	// Struct without jubako tags should work as before
	type SimpleConfig struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	data := map[string]any{
		"name":  "test",
		"value": 42,
	}

	store := New[SimpleConfig]()
	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	if config.Name != "test" {
		t.Errorf("Name = %v, want test", config.Name)
	}
	if config.Value != 42 {
		t.Errorf("Value = %v, want 42", config.Value)
	}
}

func TestDecodeWithTags_SkipField(t *testing.T) {
	ctx := context.Background()

	type ConfigWithSkip struct {
		Name    string `json:"name" jubako:"/name"`
		Ignored string `json:"ignored" jubako:"-"`
	}

	data := map[string]any{
		"name":    "test",
		"ignored": "should not be set",
	}

	store := New[ConfigWithSkip]()
	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	if config.Name != "test" {
		t.Errorf("Name = %v, want test", config.Name)
	}
	if config.Ignored != "" {
		t.Errorf("Ignored = %v, want empty string", config.Ignored)
	}
}

func TestDecodeWithTags_MissingPath(t *testing.T) {
	ctx := context.Background()

	type ConfigWithMissingPath struct {
		Exists  string `json:"exists" jubako:"/exists"`
		Missing string `json:"missing" jubako:"/does/not/exist"`
	}

	data := map[string]any{
		"exists": "value",
	}

	store := New[ConfigWithMissingPath]()
	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	if config.Exists != "value" {
		t.Errorf("Exists = %v, want value", config.Exists)
	}
	// Missing path should result in zero value
	if config.Missing != "" {
		t.Errorf("Missing = %v, want empty string", config.Missing)
	}
}

func TestDecodeWithTags_NestedStruct(t *testing.T) {
	ctx := context.Background()

	type DatabaseConfig struct {
		Host string `json:"host" jubako:"/database/host"`
		Port int    `json:"port" jubako:"/database/port"`
		Name string `json:"name" jubako:"/database/name"`
	}

	type ServerConfig struct {
		Host string `json:"host" jubako:"/server/host"`
		Port int    `json:"port" jubako:"/server/port"`
	}

	type AppConfig struct {
		Server   ServerConfig   `json:"server"`
		Database DatabaseConfig `json:"database"`
	}

	data := map[string]any{
		"server": map[string]any{
			"host": "localhost",
			"port": 8080,
		},
		"database": map[string]any{
			"host": "db.example.com",
			"port": 5432,
			"name": "mydb",
		},
	}

	store := New[AppConfig]()
	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	if config.Server.Host != "localhost" {
		t.Errorf("Server.Host = %v, want localhost", config.Server.Host)
	}
	if config.Server.Port != 8080 {
		t.Errorf("Server.Port = %v, want 8080", config.Server.Port)
	}
	if config.Database.Host != "db.example.com" {
		t.Errorf("Database.Host = %v, want db.example.com", config.Database.Host)
	}
	if config.Database.Port != 5432 {
		t.Errorf("Database.Port = %v, want 5432", config.Database.Port)
	}
	if config.Database.Name != "mydb" {
		t.Errorf("Database.Name = %v, want mydb", config.Database.Name)
	}
}

func TestDecodeWithTags_Slice(t *testing.T) {
	ctx := context.Background()

	type ConfigWithSlice struct {
		Hosts []string `json:"hosts" jubako:"/servers/hosts"`
		Ports []int    `json:"ports" jubako:"/servers/ports"`
	}

	data := map[string]any{
		"servers": map[string]any{
			"hosts": []any{"host1", "host2", "host3"},
			"ports": []any{8080, 8081, 8082},
		},
	}

	store := New[ConfigWithSlice]()
	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	expectedHosts := []string{"host1", "host2", "host3"}
	if len(config.Hosts) != len(expectedHosts) {
		t.Fatalf("Hosts length = %d, want %d", len(config.Hosts), len(expectedHosts))
	}
	for i, host := range config.Hosts {
		if host != expectedHosts[i] {
			t.Errorf("Hosts[%d] = %v, want %v", i, host, expectedHosts[i])
		}
	}

	expectedPorts := []int{8080, 8081, 8082}
	if len(config.Ports) != len(expectedPorts) {
		t.Fatalf("Ports length = %d, want %d", len(config.Ports), len(expectedPorts))
	}
	for i, port := range config.Ports {
		if port != expectedPorts[i] {
			t.Errorf("Ports[%d] = %v, want %v", i, port, expectedPorts[i])
		}
	}
}

func TestDecodeWithTags_Map(t *testing.T) {
	ctx := context.Background()

	type ConfigWithMap struct {
		Labels map[string]string `json:"labels" jubako:"/metadata/labels"`
	}

	data := map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{
				"env":     "production",
				"version": "v1",
			},
		},
	}

	store := New[ConfigWithMap]()
	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	if config.Labels["env"] != "production" {
		t.Errorf("Labels[env] = %v, want production", config.Labels["env"])
	}
	if config.Labels["version"] != "v1" {
		t.Errorf("Labels[version] = %v, want v1", config.Labels["version"])
	}
}

func TestMappingTable(t *testing.T) {
	type ServerConfig struct {
		Host            string `json:"host" jubako:"/server/host"`
		Port            int    `json:"port" jubako:"/server/port"`
		HTTPReadTimeout int    `json:"http_read_timeout" jubako:"/server/http/read_timeout"`
		Ignored         string `json:"ignored" jubako:"-"`
	}

	store := New[ServerConfig]()

	// Check HasMappings
	if !store.HasMappings() {
		t.Error("HasMappings() = false, want true")
	}

	// Check Schema
	schema := store.Schema()
	if schema == nil {
		t.Fatal("Schema() returned nil")
	}
	table := schema.Table
	if table == nil {
		t.Fatal("Schema().Table returned nil")
	}

	t.Logf("Schema:\n%s", schema)

	// Verify mappings programmatically
	if len(table.Mappings) != 4 {
		t.Errorf("len(Mappings) = %d, want 4", len(table.Mappings))
	}

	// Check individual mappings
	found := make(map[string]bool)
	for _, m := range table.Mappings {
		found[m.FieldKey] = true
		switch m.FieldKey {
		case "host":
			if m.SourcePath != "/server/host" {
				t.Errorf("host.SourcePath = %q, want /server/host", m.SourcePath)
			}
		case "port":
			if m.SourcePath != "/server/port" {
				t.Errorf("port.SourcePath = %q, want /server/port", m.SourcePath)
			}
		case "http_read_timeout":
			if m.SourcePath != "/server/http/read_timeout" {
				t.Errorf("http_read_timeout.SourcePath = %q, want /server/http/read_timeout", m.SourcePath)
			}
		case "ignored":
			if !m.Skipped {
				t.Error("ignored.Skipped = false, want true")
			}
		}
	}

	// Verify all expected fields are present
	for _, key := range []string{"host", "port", "http_read_timeout", "ignored"} {
		if !found[key] {
			t.Errorf("missing mapping for %q", key)
		}
	}
}

func TestMappingInfo_NoMappings(t *testing.T) {
	// Struct without jubako tags
	type SimpleConfig struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	store := New[SimpleConfig]()

	// Check HasMappings
	if store.HasMappings() {
		t.Error("HasMappings() = true, want false")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDecodeWithTags_NestedStructWithAbsolutePaths(t *testing.T) {
	ctx := context.Background()

	// Nested struct with jubako tags using absolute paths from root
	// This tests that nested struct fields can remap from paths outside their JSON scope
	type DatabaseConfig struct {
		Host string `json:"host" jubako:"/config/db/connection_host"`
		Port int    `json:"port" jubako:"/config/db/connection_port"`
	}

	type AppConfig struct {
		Database DatabaseConfig `json:"database"`
	}

	// Source structure is completely different from target structure
	data := map[string]any{
		"config": map[string]any{
			"db": map[string]any{
				"connection_host": "db.example.com",
				"connection_port": 5432,
			},
		},
	}

	store := New[AppConfig]()
	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	if config.Database.Host != "db.example.com" {
		t.Errorf("Database.Host = %q, want \"db.example.com\"", config.Database.Host)
	}
	if config.Database.Port != 5432 {
		t.Errorf("Database.Port = %d, want 5432", config.Database.Port)
	}
}

func TestDecodeWithTags_SliceElementRelativePaths(t *testing.T) {
	ctx := context.Background()

	// Struct for slice elements with relative paths
	type Node struct {
		// Relative paths (no leading /) - resolved from each element
		Host string `json:"host" jubako:"connection/host"`
		Port int    `json:"port" jubako:"connection/port"`
		// Absolute path - resolved from root
		DefaultTimeout int `json:"default_timeout" jubako:"/defaults/timeout"`
	}

	type ClusterConfig struct {
		Nodes []Node `json:"nodes"`
	}

	data := map[string]any{
		"defaults": map[string]any{
			"timeout": 30,
		},
		"nodes": []any{
			map[string]any{
				"connection": map[string]any{
					"host": "node1.example.com",
					"port": 5432,
				},
			},
			map[string]any{
				"connection": map[string]any{
					"host": "node2.example.com",
					"port": 5433,
				},
			},
		},
	}

	store := New[ClusterConfig]()
	t.Logf("Schema:\n%s", store.Schema())

	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	if len(config.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d, want 2", len(config.Nodes))
	}

	// Check first node
	if config.Nodes[0].Host != "node1.example.com" {
		t.Errorf("Nodes[0].Host = %q, want \"node1.example.com\"", config.Nodes[0].Host)
	}
	if config.Nodes[0].Port != 5432 {
		t.Errorf("Nodes[0].Port = %d, want 5432", config.Nodes[0].Port)
	}
	if config.Nodes[0].DefaultTimeout != 30 {
		t.Errorf("Nodes[0].DefaultTimeout = %d, want 30", config.Nodes[0].DefaultTimeout)
	}

	// Check second node
	if config.Nodes[1].Host != "node2.example.com" {
		t.Errorf("Nodes[1].Host = %q, want \"node2.example.com\"", config.Nodes[1].Host)
	}
	if config.Nodes[1].Port != 5433 {
		t.Errorf("Nodes[1].Port = %d, want 5433", config.Nodes[1].Port)
	}
	if config.Nodes[1].DefaultTimeout != 30 {
		t.Errorf("Nodes[1].DefaultTimeout = %d, want 30", config.Nodes[1].DefaultTimeout)
	}
}

func TestDecodeWithTags_MapValueRelativePaths(t *testing.T) {
	ctx := context.Background()

	// Struct for map values with relative paths
	type ServiceConfig struct {
		// Relative path - resolved from each map value
		Endpoint string `json:"endpoint" jubako:"settings/endpoint"`
		// Absolute path - resolved from root
		DefaultRetries int `json:"default_retries" jubako:"/defaults/retries"`
	}

	type Config struct {
		Services map[string]ServiceConfig `json:"services"`
	}

	data := map[string]any{
		"defaults": map[string]any{
			"retries": 3,
		},
		"services": map[string]any{
			"api": map[string]any{
				"settings": map[string]any{
					"endpoint": "https://api.example.com",
				},
			},
			"web": map[string]any{
				"settings": map[string]any{
					"endpoint": "https://web.example.com",
				},
			},
		},
	}

	store := New[Config]()
	t.Logf("Schema:\n%s", store.Schema())

	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	if len(config.Services) != 2 {
		t.Fatalf("len(Services) = %d, want 2", len(config.Services))
	}

	// Check api service
	if config.Services["api"].Endpoint != "https://api.example.com" {
		t.Errorf("Services[api].Endpoint = %q, want \"https://api.example.com\"", config.Services["api"].Endpoint)
	}
	if config.Services["api"].DefaultRetries != 3 {
		t.Errorf("Services[api].DefaultRetries = %d, want 3", config.Services["api"].DefaultRetries)
	}

	// Check web service
	if config.Services["web"].Endpoint != "https://web.example.com" {
		t.Errorf("Services[web].Endpoint = %q, want \"https://web.example.com\"", config.Services["web"].Endpoint)
	}
	if config.Services["web"].DefaultRetries != 3 {
		t.Errorf("Services[web].DefaultRetries = %d, want 3", config.Services["web"].DefaultRetries)
	}
}

func TestDecodeWithTags_ExplicitRelativePath(t *testing.T) {
	ctx := context.Background()

	// Test explicit "./" prefix for relative paths
	type Item struct {
		Name  string `json:"name" jubako:"./info/name"`   // explicit relative
		Value int    `json:"value" jubako:"info/value"`   // implicit relative
		ID    string `json:"id" jubako:"/global/item_id"` // absolute
	}

	type Config struct {
		Items []Item `json:"items"`
	}

	data := map[string]any{
		"global": map[string]any{
			"item_id": "global-id",
		},
		"items": []any{
			map[string]any{
				"info": map[string]any{
					"name":  "item1",
					"value": 100,
				},
			},
		},
	}

	store := New[Config]()
	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	if len(config.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(config.Items))
	}

	if config.Items[0].Name != "item1" {
		t.Errorf("Items[0].Name = %q, want \"item1\"", config.Items[0].Name)
	}
	if config.Items[0].Value != 100 {
		t.Errorf("Items[0].Value = %d, want 100", config.Items[0].Value)
	}
	if config.Items[0].ID != "global-id" {
		t.Errorf("Items[0].ID = %q, want \"global-id\"", config.Items[0].ID)
	}
}

func TestDecodeWithTags_TypeConversion(t *testing.T) {
	ctx := context.Background()

	// Note: Standard JSON decoder has limited type conversion
	// Type conversion is the decoder's responsibility, not jubako's
	// This test verifies that jubako correctly remaps paths regardless of types
	type ConfigWithTypes struct {
		IntVal   int     `json:"int_val" jubako:"/values/integer"`
		FloatVal float64 `json:"float_val" jubako:"/values/float"`
		StrVal   string  `json:"str_val" jubako:"/values/string"`
		BoolVal  bool    `json:"bool_val" jubako:"/values/boolean"`
	}

	data := map[string]any{
		"values": map[string]any{
			"integer": 42,
			"float":   3.14,
			"string":  "hello",
			"boolean": true,
		},
	}

	store := New[ConfigWithTypes]()
	err := store.Add(mapdata.New("config", data))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	config := store.Get()

	if config.IntVal != 42 {
		t.Errorf("IntVal = %v, want 42", config.IntVal)
	}
	if config.FloatVal != 3.14 {
		t.Errorf("FloatVal = %v, want 3.14", config.FloatVal)
	}
	if config.StrVal != "hello" {
		t.Errorf("StrVal = %v, want hello", config.StrVal)
	}
	if config.BoolVal != true {
		t.Errorf("BoolVal = %v, want true", config.BoolVal)
	}
}
