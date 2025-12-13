package mapdata

import (
	"context"
	"reflect"
	"testing"
)

func TestNew(t *testing.T) {
	t.Run("creates layer with data", func(t *testing.T) {
		data := map[string]any{"key": "value"}
		l := New("test", data)

		if l.Name() != "test" {
			t.Errorf("Name() = %q, want %q", l.Name(), "test")
		}
	})

	t.Run("creates layer with nil data", func(t *testing.T) {
		l := New("test", nil)

		if l.Name() != "test" {
			t.Errorf("Name() = %q, want %q", l.Name(), "test")
		}
	})
}

func TestLayer_Load(t *testing.T) {
	ctx := context.Background()

	t.Run("loads data into document", func(t *testing.T) {
		data := map[string]any{
			"server": map[string]any{
				"host": "localhost",
				"port": 8080,
			},
		}
		l := New("test", data)

		doc, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if doc == nil {
			t.Fatal("Load() returned nil document")
		}

		// Verify document content
		val, ok := doc.Get("/server/host")
		if !ok {
			t.Error("Get(/server/host) not found")
		}
		if val != "localhost" {
			t.Errorf("Get(/server/host) = %v, want %q", val, "localhost")
		}
	})

	t.Run("deep copies data", func(t *testing.T) {
		data := map[string]any{"key": "original"}
		l := New("test", data)

		_, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Modify original data
		data["key"] = "modified"

		// Document should still have original value
		val, _ := l.Document().Get("/key")
		if val != "original" {
			t.Errorf("Document data was modified by external change: got %v", val)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		l := New("test", map[string]any{"key": "value"})

		ctx, cancel := context.WithCancel(ctx)
		cancel()

		_, err := l.Load(ctx)
		if err == nil {
			t.Error("Load() should return error with canceled context")
		}
	})
}

func TestLayer_Document(t *testing.T) {
	t.Run("returns nil before load", func(t *testing.T) {
		l := New("test", map[string]any{"key": "value"})

		if l.Document() != nil {
			t.Error("Document() should return nil before Load()")
		}
	})

	t.Run("returns document after load", func(t *testing.T) {
		ctx := context.Background()
		l := New("test", map[string]any{"key": "value"})

		l.Load(ctx)

		if l.Document() == nil {
			t.Error("Document() should return document after Load()")
		}
	})
}

func TestLayer_Save(t *testing.T) {
	ctx := context.Background()
	l := New("test", map[string]any{"key": "value"})

	_, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Save should succeed (no-op for in-memory data)
	err = l.Save(ctx)
	if err != nil {
		t.Errorf("Save() error = %v, want nil", err)
	}

	// Save should respect context cancellation
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	err = l.Save(canceledCtx)
	if err == nil {
		t.Error("Save() should return error with canceled context")
	}
}

func TestLayer_CanSave(t *testing.T) {
	l := New("test", map[string]any{"key": "value"})

	if !l.CanSave() {
		t.Error("CanSave() should return true")
	}
}

func TestDocument_Get(t *testing.T) {
	data := map[string]any{
		"server": map[string]any{
			"host": "localhost",
			"port": 8080,
		},
		"debug": true,
	}
	doc := NewDocument(data)

	tests := []struct {
		path   string
		want   any
		wantOk bool
	}{
		{"/server/host", "localhost", true},
		{"/server/port", 8080, true},
		{"/debug", true, true},
		{"/server", map[string]any{"host": "localhost", "port": 8080}, true},
		{"/nonexistent", nil, false},
		{"/server/nonexistent", nil, false},
		{"", data, true},
		{"/", data, true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, ok := doc.Get(tt.path)
			if ok != tt.wantOk {
				t.Errorf("Get(%q) ok = %v, want %v", tt.path, ok, tt.wantOk)
			}
			if tt.wantOk && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDocument_Set(t *testing.T) {
	t.Run("sets existing value", func(t *testing.T) {
		doc := NewDocument(map[string]any{"key": "old"})

		err := doc.Set("/key", "new")
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		val, _ := doc.Get("/key")
		if val != "new" {
			t.Errorf("Get(/key) = %v, want %q", val, "new")
		}
	})

	t.Run("creates nested path", func(t *testing.T) {
		doc := NewDocument(map[string]any{})

		err := doc.Set("/server/host", "localhost")
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		val, ok := doc.Get("/server/host")
		if !ok {
			t.Error("Get(/server/host) not found")
		}
		if val != "localhost" {
			t.Errorf("Get(/server/host) = %v, want %q", val, "localhost")
		}
	})

	t.Run("rejects empty path", func(t *testing.T) {
		doc := NewDocument(map[string]any{})

		err := doc.Set("", "value")
		if err == nil {
			t.Error("Set() should return error for empty path")
		}
	})
}

func TestDocument_Delete(t *testing.T) {
	t.Run("deletes existing key", func(t *testing.T) {
		doc := NewDocument(map[string]any{"key": "value", "other": "data"})

		err := doc.Delete("/key")
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, ok := doc.Get("/key")
		if ok {
			t.Error("Get(/key) should return false after delete")
		}

		// Other key should still exist
		val, ok := doc.Get("/other")
		if !ok || val != "data" {
			t.Error("Delete should not affect other keys")
		}
	})

	t.Run("deletes nested key", func(t *testing.T) {
		doc := NewDocument(map[string]any{
			"server": map[string]any{
				"host": "localhost",
				"port": 8080,
			},
		})

		err := doc.Delete("/server/port")
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, ok := doc.Get("/server/port")
		if ok {
			t.Error("Get(/server/port) should return false after delete")
		}

		// Host should still exist
		val, ok := doc.Get("/server/host")
		if !ok || val != "localhost" {
			t.Error("Delete should not affect sibling keys")
		}
	})

	t.Run("idempotent for nonexistent key", func(t *testing.T) {
		doc := NewDocument(map[string]any{"key": "value"})

		err := doc.Delete("/nonexistent")
		if err != nil {
			t.Errorf("Delete() should be idempotent, got error = %v", err)
		}
	})

	t.Run("rejects empty path", func(t *testing.T) {
		doc := NewDocument(map[string]any{})

		err := doc.Delete("")
		if err == nil {
			t.Error("Delete() should return error for empty path")
		}
	})
}

func TestDocument_Format(t *testing.T) {
	doc := NewDocument(map[string]any{})

	if doc.Format() != "mapdata" {
		t.Errorf("Format() = %q, want %q", doc.Format(), "mapdata")
	}
}

func TestDocument_Marshal(t *testing.T) {
	doc := NewDocument(map[string]any{"key": "value"})

	_, err := doc.Marshal()
	if err == nil {
		t.Error("Marshal() should return error")
	}
}

func TestDocument_MarshalTestData(t *testing.T) {
	doc := NewDocument(map[string]any{})

	_, err := doc.MarshalTestData(map[string]any{"key": "value"})
	if err == nil {
		t.Error("MarshalTestData() should return error")
	}
}

func TestDeepCopy(t *testing.T) {
	t.Run("copies nested maps", func(t *testing.T) {
		original := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"value": "deep",
				},
			},
		}

		copied := deepCopyMap(original)

		// Modify original
		original["level1"].(map[string]any)["level2"].(map[string]any)["value"] = "modified"

		// Copied should be unchanged
		val := copied["level1"].(map[string]any)["level2"].(map[string]any)["value"]
		if val != "deep" {
			t.Errorf("Deep copy was affected by modification: got %v", val)
		}
	})

	t.Run("copies slices", func(t *testing.T) {
		original := map[string]any{
			"items": []any{"a", "b", "c"},
		}

		copied := deepCopyMap(original)

		// Modify original slice
		original["items"].([]any)[0] = "modified"

		// Copied should be unchanged
		val := copied["items"].([]any)[0]
		if val != "a" {
			t.Errorf("Deep copy slice was affected by modification: got %v", val)
		}
	})

	t.Run("handles nil", func(t *testing.T) {
		copied := deepCopyMap(nil)
		if copied != nil {
			t.Error("deepCopyMap(nil) should return nil")
		}
	})
}
