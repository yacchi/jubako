package jubako

import (
	"context"
	"testing"
	"time"

	"github.com/yacchi/jubako/layer/mapdata"
)

func TestStore_Set(t *testing.T) {
	ctx := context.Background()

	t.Run("set single value with Int", func(t *testing.T) {
		store := New[testConfig]()
		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		err = store.Set("test", Int("/port", 9000))
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Port != 9000 {
			t.Errorf("Port = %d, want 9000", cfg.Port)
		}
	})

	t.Run("set single value with String", func(t *testing.T) {
		store := New[testConfig]()
		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		err = store.Set("test", String("/host", "newhost"))
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Host != "newhost" {
			t.Errorf("Host = %q, want %q", cfg.Host, "newhost")
		}
	})

	t.Run("set multiple values", func(t *testing.T) {
		store := New[testConfig]()
		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		err = store.Set("test",
			String("/host", "newhost"),
			Int("/port", 9000),
		)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Host != "newhost" {
			t.Errorf("Host = %q, want %q", cfg.Host, "newhost")
		}
		if cfg.Port != 9000 {
			t.Errorf("Port = %d, want 9000", cfg.Port)
		}
	})

	t.Run("set with Path grouping", func(t *testing.T) {
		type nestedConfig struct {
			Server struct {
				Host string `json:"host"`
				Port int    `json:"port"`
			} `json:"server"`
		}

		store := New[nestedConfig]()
		err := store.Add(mapdata.New("test", map[string]any{
			"server": map[string]any{
				"host": "localhost",
				"port": 8080,
			},
		}))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		err = store.Set("test", Path("/server",
			String("host", "newhost"),
			Int("port", 9000),
		))
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Server.Host != "newhost" {
			t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "newhost")
		}
		if cfg.Server.Port != 9000 {
			t.Errorf("Server.Port = %d, want 9000", cfg.Server.Port)
		}
	})

	t.Run("set with Map", func(t *testing.T) {
		type mapConfig struct {
			Settings map[string]any `json:"settings"`
		}

		store := New[mapConfig]()
		err := store.Add(mapdata.New("test", map[string]any{
			"settings": map[string]any{},
		}))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		err = store.Set("test", Map("/settings", map[string]any{
			"key1": "value1",
			"key2": 42,
		}))
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Settings["key1"] != "value1" {
			t.Errorf("Settings[key1] = %v, want %q", cfg.Settings["key1"], "value1")
		}
		if cfg.Settings["key2"] != float64(42) { // JSON numbers become float64
			t.Errorf("Settings[key2] = %v, want 42", cfg.Settings["key2"])
		}
	})

	t.Run("set with Struct", func(t *testing.T) {
		type credConfig struct {
			Credential struct {
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"credential"`
		}

		store := New[credConfig]()
		err := store.Add(mapdata.New("test", map[string]any{
			"credential": map[string]any{},
		}))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		cred := struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}{
			Username: "admin",
			Password: "secret",
		}

		err = store.Set("test", Struct("/credential", cred))
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Credential.Username != "admin" {
			t.Errorf("Credential.Username = %q, want %q", cfg.Credential.Username, "admin")
		}
		if cfg.Credential.Password != "secret" {
			t.Errorf("Credential.Password = %q, want %q", cfg.Credential.Password, "secret")
		}
	})

	t.Run("set with SkipZeroValues", func(t *testing.T) {
		store := New[testConfig]()
		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Port=0 should be skipped
		err = store.Set("test",
			String("/host", "newhost"),
			Int("/port", 0),
			SkipZeroValues(),
		)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Host != "newhost" {
			t.Errorf("Host = %q, want %q", cfg.Host, "newhost")
		}
		if cfg.Port != 8080 { // Should remain unchanged
			t.Errorf("Port = %d, want 8080 (unchanged)", cfg.Port)
		}
	})

	t.Run("set with DeleteNilValue", func(t *testing.T) {
		type optConfig struct {
			Host    string `json:"host"`
			Port    int    `json:"port"`
			Comment string `json:"comment"`
		}

		store := New[optConfig]()
		err := store.Add(mapdata.New("test", map[string]any{
			"host":    "localhost",
			"port":    8080,
			"comment": "test comment",
		}))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// comment=nil should delete the field
		err = store.Set("test",
			String("/host", "newhost"),
			Value("/comment", nil),
			DeleteNilValue(),
		)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		cfg := store.Get()
		if cfg.Host != "newhost" {
			t.Errorf("Host = %q, want %q", cfg.Host, "newhost")
		}
		if cfg.Comment != "" { // Should be empty (deleted)
			t.Errorf("Comment = %q, want empty (deleted)", cfg.Comment)
		}
	})

	t.Run("set notifies subscribers", func(t *testing.T) {
		store := New[testConfig]()
		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		var notified bool
		var receivedPort int
		unsubscribe := store.Subscribe(func(cfg testConfig) {
			notified = true
			receivedPort = cfg.Port
		})
		defer unsubscribe()

		err = store.Set("test", Int("/port", 9000))
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		if !notified {
			t.Error("Subscribers were not notified after Set()")
		}
		if receivedPort != 9000 {
			t.Errorf("Subscriber received Port = %d, want 9000", receivedPort)
		}
	})

	t.Run("set empty options does nothing", func(t *testing.T) {
		store := New[testConfig]()
		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost", "port": 8080}))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if err := store.Load(ctx); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		err = store.Set("test")
		if err != nil {
			t.Fatalf("Set() with no options should not error: %v", err)
		}

		cfg := store.Get()
		if cfg.Port != 8080 {
			t.Errorf("Port = %d, want 8080 (unchanged)", cfg.Port)
		}
	})

	t.Run("set to non-existent layer fails", func(t *testing.T) {
		store := New[testConfig]()

		err := store.Set("nonexistent", Int("/port", 9000))
		if err == nil {
			t.Error("Set() should return error for non-existent layer")
		}
	})

	t.Run("set before load fails", func(t *testing.T) {
		store := New[testConfig]()
		err := store.Add(mapdata.New("test", map[string]any{"host": "localhost"}))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		err = store.Set("test", Int("/port", 9000))
		if err == nil {
			t.Error("Set() should return error before Load()")
		}
	})
}

func TestSetOption_Value(t *testing.T) {
	t.Run("Value accepts various types", func(t *testing.T) {
		cfg := &setConfig{}

		Value("/string", "test")(cfg)
		Value("/int", 42)(cfg)
		Value("/float", 3.14)(cfg)
		Value("/bool", true)(cfg)

		if len(cfg.patches) != 4 {
			t.Errorf("patches count = %d, want 4", len(cfg.patches))
		}
	})
}

func TestSetOption_Path(t *testing.T) {
	t.Run("Path adds prefix to children", func(t *testing.T) {
		cfg := &setConfig{}

		Path("/server",
			Int("port", 8080),
			String("host", "localhost"),
		)(cfg)

		if len(cfg.patches) != 2 {
			t.Fatalf("patches count = %d, want 2", len(cfg.patches))
		}

		// Check paths have prefix
		paths := make(map[string]bool)
		for _, pv := range cfg.patches {
			paths[pv.path] = true
		}

		if !paths["/server/port"] {
			t.Error("expected /server/port in patches")
		}
		if !paths["/server/host"] {
			t.Error("expected /server/host in patches")
		}
	})

	t.Run("nested Path", func(t *testing.T) {
		cfg := &setConfig{}

		Path("/a",
			Path("b",
				Int("c", 1),
			),
		)(cfg)

		if len(cfg.patches) != 1 {
			t.Fatalf("patches count = %d, want 1", len(cfg.patches))
		}

		if cfg.patches[0].path != "/a/b/c" {
			t.Errorf("path = %q, want %q", cfg.patches[0].path, "/a/b/c")
		}
	})
}

func TestSetOption_Struct(t *testing.T) {
	t.Run("expands struct fields", func(t *testing.T) {
		cfg := &setConfig{}

		data := struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}{
			Name:  "test",
			Count: 42,
		}

		Struct("/data", data)(cfg)

		if len(cfg.patches) != 2 {
			t.Fatalf("patches count = %d, want 2", len(cfg.patches))
		}

		paths := make(map[string]any)
		for _, pv := range cfg.patches {
			paths[pv.path] = pv.value
		}

		if paths["/data/name"] != "test" {
			t.Errorf("/data/name = %v, want %q", paths["/data/name"], "test")
		}
		if paths["/data/count"] != 42 {
			t.Errorf("/data/count = %v, want 42", paths["/data/count"])
		}
	})

	t.Run("handles pointer to struct", func(t *testing.T) {
		cfg := &setConfig{}

		data := &struct {
			Name string `json:"name"`
		}{
			Name: "test",
		}

		Struct("/data", data)(cfg)

		if len(cfg.patches) != 1 {
			t.Fatalf("patches count = %d, want 1", len(cfg.patches))
		}

		if cfg.patches[0].value != "test" {
			t.Errorf("value = %v, want %q", cfg.patches[0].value, "test")
		}
	})

	t.Run("handles nested struct", func(t *testing.T) {
		cfg := &setConfig{}

		data := struct {
			Inner struct {
				Value string `json:"value"`
			} `json:"inner"`
		}{
			Inner: struct {
				Value string `json:"value"`
			}{
				Value: "nested",
			},
		}

		Struct("/data", data)(cfg)

		if len(cfg.patches) != 1 {
			t.Fatalf("patches count = %d, want 1", len(cfg.patches))
		}

		if cfg.patches[0].path != "/data/inner/value" {
			t.Errorf("path = %q, want %q", cfg.patches[0].path, "/data/inner/value")
		}
	})

	t.Run("handles time.Time as leaf value", func(t *testing.T) {
		cfg := &setConfig{}

		now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		data := struct {
			Name      string    `json:"name"`
			CreatedAt time.Time `json:"created_at"`
		}{
			Name:      "test",
			CreatedAt: now,
		}

		Struct("/data", data)(cfg)

		if len(cfg.patches) != 2 {
			t.Fatalf("patches count = %d, want 2", len(cfg.patches))
		}

		paths := make(map[string]any)
		for _, pv := range cfg.patches {
			paths[pv.path] = pv.value
		}

		if paths["/data/name"] != "test" {
			t.Errorf("/data/name = %v, want %q", paths["/data/name"], "test")
		}

		createdAt, ok := paths["/data/created_at"].(time.Time)
		if !ok {
			t.Fatalf("/data/created_at is not time.Time, got %T", paths["/data/created_at"])
		}
		if !createdAt.Equal(now) {
			t.Errorf("/data/created_at = %v, want %v", createdAt, now)
		}
	})

	t.Run("handles pointer to time.Time", func(t *testing.T) {
		cfg := &setConfig{}

		now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		data := struct {
			Name      string     `json:"name"`
			UpdatedAt *time.Time `json:"updated_at"`
		}{
			Name:      "test",
			UpdatedAt: &now,
		}

		Struct("/data", data)(cfg)

		if len(cfg.patches) != 2 {
			t.Fatalf("patches count = %d, want 2", len(cfg.patches))
		}

		paths := make(map[string]any)
		for _, pv := range cfg.patches {
			paths[pv.path] = pv.value
		}

		updatedAt, ok := paths["/data/updated_at"].(*time.Time)
		if !ok {
			t.Fatalf("/data/updated_at is not *time.Time, got %T", paths["/data/updated_at"])
		}
		if !updatedAt.Equal(now) {
			t.Errorf("/data/updated_at = %v, want %v", *updatedAt, now)
		}
	})
}

func TestSetOption_Map(t *testing.T) {
	t.Run("expands map entries", func(t *testing.T) {
		cfg := &setConfig{}

		Map("/settings", map[string]any{
			"key1": "value1",
			"key2": 42,
		})(cfg)

		if len(cfg.patches) != 2 {
			t.Fatalf("patches count = %d, want 2", len(cfg.patches))
		}

		paths := make(map[string]any)
		for _, pv := range cfg.patches {
			paths[pv.path] = pv.value
		}

		if paths["/settings/key1"] != "value1" {
			t.Errorf("/settings/key1 = %v, want %q", paths["/settings/key1"], "value1")
		}
		if paths["/settings/key2"] != 42 {
			t.Errorf("/settings/key2 = %v, want 42", paths["/settings/key2"])
		}
	})
}

func TestIsZeroValue(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{"nil", nil, true},
		{"empty string", "", true},
		{"non-empty string", "test", false},
		{"zero int", 0, true},
		{"non-zero int", 42, false},
		{"false bool", false, true},
		{"true bool", true, false},
		{"zero float", 0.0, true},
		{"non-zero float", 3.14, false},
		{"nil slice", []int(nil), true},
		{"empty slice", []int{}, false}, // empty slice is not nil
		{"nil map", map[string]int(nil), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isZeroValue(tt.value)
			if got != tt.want {
				t.Errorf("isZeroValue(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
