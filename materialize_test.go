package jubako

import (
	"context"
	"reflect"
	"testing"

	"github.com/yacchi/jubako/decoder"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/layer/mapdata"
)

type testServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type testDatabaseConfig struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	MaxConnections int    `json:"max_connections"`
}

type testAppConfig struct {
	Server   testServerConfig   `json:"server"`
	Database testDatabaseConfig `json:"database"`
	Debug    bool               `json:"debug"`
}

func TestStore_materialize_SingleLayer(t *testing.T) {
	ctx := context.Background()
	store := New[testAppConfig]()

	data := map[string]any{
		"server": map[string]any{
			"host": "localhost",
			"port": 8080,
		},
		"database": map[string]any{
			"host":            "db.example.com",
			"port":            5432,
			"max_connections": 10,
		},
		"debug": true,
	}

	err := store.Add(mapdata.New("defaults", data), WithPriority(PriorityDefaults))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := store.Get()

	if cfg.Server.Host != "localhost" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "localhost")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 8080)
	}
	if cfg.Database.Host != "db.example.com" {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, "db.example.com")
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("Database.Port = %d, want %d", cfg.Database.Port, 5432)
	}
	if cfg.Database.MaxConnections != 10 {
		t.Errorf("Database.MaxConnections = %d, want %d", cfg.Database.MaxConnections, 10)
	}
	if cfg.Debug != true {
		t.Errorf("Debug = %v, want %v", cfg.Debug, true)
	}
}

func TestStore_materialize_MultipleLayers(t *testing.T) {
	ctx := context.Background()
	store := New[testAppConfig]()

	// Layer 1: Defaults (lowest priority)
	defaultsData := map[string]any{
		"server": map[string]any{
			"host": "localhost",
			"port": 8080,
		},
		"database": map[string]any{
			"host":            "db.example.com",
			"port":            5432,
			"max_connections": 10,
		},
		"debug": false,
	}

	// Layer 2: User config (higher priority)
	userData := map[string]any{
		"server": map[string]any{
			"port": 9000,
		},
		"database": map[string]any{
			"max_connections": 20,
		},
	}

	// Layer 3: Environment (highest priority)
	envData := map[string]any{
		"debug": true,
	}

	err := store.Add(mapdata.New("defaults", defaultsData), WithPriority(PriorityDefaults))
	if err != nil {
		t.Fatalf("Add(defaults) error = %v", err)
	}

	err = store.Add(mapdata.New("user", userData), WithPriority(PriorityUser))
	if err != nil {
		t.Fatalf("Add(user) error = %v", err)
	}

	err = store.Add(mapdata.New("env", envData), WithPriority(PriorityEnv))
	if err != nil {
		t.Fatalf("Add(env) error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := store.Get()

	// Server.Host should come from defaults (not overridden)
	if cfg.Server.Host != "localhost" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "localhost")
	}

	// Server.Port should come from user (overrides defaults)
	if cfg.Server.Port != 9000 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 9000)
	}

	// Database.Host should come from defaults (not overridden)
	if cfg.Database.Host != "db.example.com" {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, "db.example.com")
	}

	// Database.Port should come from defaults (not overridden)
	if cfg.Database.Port != 5432 {
		t.Errorf("Database.Port = %d, want %d", cfg.Database.Port, 5432)
	}

	// Database.MaxConnections should come from user (overrides defaults)
	if cfg.Database.MaxConnections != 20 {
		t.Errorf("Database.MaxConnections = %d, want %d", cfg.Database.MaxConnections, 20)
	}

	// Debug should come from env (highest priority)
	if cfg.Debug != true {
		t.Errorf("Debug = %v, want %v", cfg.Debug, true)
	}
}

func TestStore_materialize_NoLayers(t *testing.T) {
	ctx := context.Background()
	store := New[testAppConfig]()

	err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := store.Get()

	// Should return zero value
	var zero testAppConfig
	if !reflect.DeepEqual(cfg, zero) {
		t.Errorf("Get() = %+v, want zero value", cfg)
	}
}

func TestStore_materialize_EmptyLayer(t *testing.T) {
	ctx := context.Background()
	store := New[testAppConfig]()

	err := store.Add(mapdata.New("empty", map[string]any{}), WithPriority(PriorityDefaults))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := store.Get()

	// Should return zero value (empty layer doesn't provide values)
	var zero testAppConfig
	if !reflect.DeepEqual(cfg, zero) {
		t.Errorf("Get() = %+v, want zero value", cfg)
	}
}

func TestDeepMerge(t *testing.T) {
	tests := []struct {
		name string
		dst  map[string]any
		src  map[string]any
		want map[string]any
	}{
		{
			name: "merge simple values",
			dst:  map[string]any{"a": 1, "b": 2},
			src:  map[string]any{"b": 3, "c": 4},
			want: map[string]any{"a": 1, "b": 3, "c": 4},
		},
		{
			name: "merge nested maps",
			dst: map[string]any{
				"server": map[string]any{
					"host": "localhost",
					"port": 8080,
				},
			},
			src: map[string]any{
				"server": map[string]any{
					"port": 9000,
				},
			},
			want: map[string]any{
				"server": map[string]any{
					"host": "localhost",
					"port": 9000,
				},
			},
		},
		{
			name: "merge deeply nested maps",
			dst: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": 1,
						"d": 2,
					},
				},
			},
			src: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"d": 3,
						"e": 4,
					},
				},
			},
			want: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": 1,
						"d": 3,
						"e": 4,
					},
				},
			},
		},
		{
			name: "replace non-map with map",
			dst:  map[string]any{"a": "value"},
			src:  map[string]any{"a": map[string]any{"b": 1}},
			want: map[string]any{"a": map[string]any{"b": 1}},
		},
		{
			name: "replace map with non-map",
			dst:  map[string]any{"a": map[string]any{"b": 1}},
			src:  map[string]any{"a": "value"},
			want: map[string]any{"a": "value"},
		},
		{
			name: "merge with nil value",
			dst:  map[string]any{"a": 1},
			src:  map[string]any{"a": nil},
			want: map[string]any{"a": nil},
		},
		{
			name: "merge empty maps",
			dst:  map[string]any{},
			src:  map[string]any{},
			want: map[string]any{},
		},
		{
			name: "merge into empty map",
			dst:  map[string]any{},
			src:  map[string]any{"a": 1, "b": 2},
			want: map[string]any{"a": 1, "b": 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of dst to avoid modifying the test case
			dst := make(map[string]any)
			for k, v := range tt.dst {
				dst[k] = v
			}

			deepMerge(dst, tt.src)

			if !reflect.DeepEqual(dst, tt.want) {
				t.Errorf("deepMerge() = %v, want %v", dst, tt.want)
			}
		})
	}
}

func TestDecodeMap(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		want    testAppConfig
		wantErr bool
	}{
		{
			name: "valid map",
			input: map[string]any{
				"server": map[string]any{
					"host": "localhost",
					"port": 8080,
				},
				"database": map[string]any{
					"host":            "db.example.com",
					"port":            5432,
					"max_connections": 10,
				},
				"debug": true,
			},
			want: testAppConfig{
				Server: testServerConfig{
					Host: "localhost",
					Port: 8080,
				},
				Database: testDatabaseConfig{
					Host:           "db.example.com",
					Port:           5432,
					MaxConnections: 10,
				},
				Debug: true,
			},
			wantErr: false,
		},
		{
			name:    "empty map",
			input:   map[string]any{},
			want:    testAppConfig{},
			wantErr: false,
		},
		{
			name: "partial map",
			input: map[string]any{
				"server": map[string]any{
					"host": "localhost",
				},
			},
			want: testAppConfig{
				Server: testServerConfig{
					Host: "localhost",
					Port: 0, // Zero value
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result testAppConfig
			err := decoder.JSON(tt.input, &result)

			if (err != nil) != tt.wantErr {
				t.Errorf("decoder.JSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !reflect.DeepEqual(result, tt.want) {
				t.Errorf("decoder.JSON() = %+v, want %+v", result, tt.want)
			}
		})
	}
}

func TestStore_materialize_SubscribersNotified(t *testing.T) {
	ctx := context.Background()
	store := New[testAppConfig]()

	var notified bool
	var receivedConfig testAppConfig

	unsubscribe := store.Subscribe(func(cfg testAppConfig) {
		notified = true
		receivedConfig = cfg
	})
	defer unsubscribe()

	data := map[string]any{
		"server": map[string]any{
			"host": "localhost",
			"port": 8080,
		},
	}

	err := store.Add(mapdata.New("test", data), WithPriority(PriorityDefaults))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !notified {
		t.Error("Subscribers were not notified after materialize")
	}

	if receivedConfig.Server.Host != "localhost" {
		t.Errorf("Subscriber received Server.Host = %q, want %q", receivedConfig.Server.Host, "localhost")
	}
	if receivedConfig.Server.Port != 8080 {
		t.Errorf("Subscriber received Server.Port = %d, want %d", receivedConfig.Server.Port, 8080)
	}
}

func TestStore_materialize_PriorityOrder(t *testing.T) {
	ctx := context.Background()
	store := New[testAppConfig]()

	// Add layers in reverse priority order to ensure sorting works
	// Port values match Priority constant values (10, 20, 30, 40) for clarity
	layers := []struct {
		name      layer.Name
		priority  layer.Priority
		data      map[string]any
		portValue int
	}{
		{"flags", PriorityFlags, map[string]any{"server": map[string]any{"port": 40}}, 40},
		{"defaults", PriorityDefaults, map[string]any{"server": map[string]any{"port": 0}}, 0},
		{"env", PriorityEnv, map[string]any{"server": map[string]any{"port": 30}}, 30},
		{"user", PriorityUser, map[string]any{"server": map[string]any{"port": 10}}, 10},
		{"project", PriorityProject, map[string]any{"server": map[string]any{"port": 20}}, 20},
	}

	for _, l := range layers {
		err := store.Add(mapdata.New(l.name, l.data), WithPriority(l.priority))
		if err != nil {
			t.Fatalf("Add(%q) error = %v", l.name, err)
		}
	}

	err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := store.Get()

	// The highest priority (PriorityFlags) should win
	if cfg.Server.Port != 40 {
		t.Errorf("Server.Port = %d, want 40 (from flags layer)", cfg.Server.Port)
	}
}

func BenchmarkStore_materialize(b *testing.B) {
	ctx := context.Background()
	data := map[string]any{
		"server": map[string]any{
			"host": "localhost",
			"port": 8080,
		},
		"database": map[string]any{
			"host":            "db.example.com",
			"port":            5432,
			"max_connections": 10,
		},
		"debug": true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store := New[testAppConfig]()
		store.Add(mapdata.New("defaults", data), WithPriority(PriorityDefaults))
		store.Load(ctx)
	}
}

func BenchmarkDeepMerge(b *testing.B) {
	dst := map[string]any{
		"server": map[string]any{
			"host": "localhost",
			"port": 8080,
		},
		"database": map[string]any{
			"host": "db.example.com",
			"port": 5432,
		},
	}

	src := map[string]any{
		"server": map[string]any{
			"port": 9000,
		},
		"debug": true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Make a copy to avoid modifying the original
		dstCopy := make(map[string]any)
		for k, v := range dst {
			dstCopy[k] = v
		}
		deepMerge(dstCopy, src)
	}
}
