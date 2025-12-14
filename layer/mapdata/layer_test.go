package mapdata

import (
	"context"
	"reflect"
	"testing"

	"github.com/yacchi/jubako/jktest"
	"github.com/yacchi/jubako/layer"
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

	t.Run("deep copies input data", func(t *testing.T) {
		data := map[string]any{"key": "original"}
		l := New("test", data)

		// Modify original data
		data["key"] = "modified"

		// Layer data should still have original value
		layerData := l.Data()
		if layerData["key"] != "original" {
			t.Errorf("Layer data was modified by external change: got %v", layerData["key"])
		}
	})
}

func TestLayer_Load(t *testing.T) {
	ctx := context.Background()

	t.Run("returns deep copy", func(t *testing.T) {
		data := map[string]any{"key": "original"}
		l := New("test", data)

		result, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Modify result
		result["key"] = "modified"

		// Original layer data should be unchanged
		layerData := l.Data()
		if layerData["key"] != "original" {
			t.Errorf("Layer data was modified by changing Load() result: got %v", layerData["key"])
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

func TestLayer_Data(t *testing.T) {
	t.Run("returns deep copy", func(t *testing.T) {
		l := New("test", map[string]any{"key": "original"})

		data := l.Data()
		data["key"] = "modified"

		// Original should be unchanged
		if l.Data()["key"] != "original" {
			t.Error("Data() should return a deep copy")
		}
	})
}

func TestLayer_Save(t *testing.T) {
	ctx := context.Background()

	t.Run("respects context cancellation", func(t *testing.T) {
		l := New("test", map[string]any{"key": "value"})
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()
		err := l.Save(canceledCtx, nil)
		if err == nil {
			t.Error("Save() should return error with canceled context")
		}
	})
}

func TestLayer_CanSave(t *testing.T) {
	l := New("test", map[string]any{"key": "value"})

	if !l.CanSave() {
		t.Error("CanSave() should return true")
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

		l := New("test", original)
		copied := l.Data()

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

		l := New("test", original)
		copied := l.Data()

		// Modify original slice
		original["items"].([]any)[0] = "modified"

		// Copied should be unchanged
		val := copied["items"].([]any)[0]
		if val != "a" {
			t.Errorf("Deep copy slice was affected by modification: got %v", val)
		}
	})
}

func TestLayer_EmptyData(t *testing.T) {
	l := New("test", nil)
	ctx := context.Background()

	result, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !reflect.DeepEqual(result, map[string]any{}) {
		t.Errorf("Load() = %v, want empty map", result)
	}
}

// TestLayer_Compliance runs the standard jktest compliance tests.
func TestLayer_Compliance(t *testing.T) {
	factory := func(data map[string]any) layer.Layer {
		return New("test", data)
	}
	jktest.NewLayerTester(t, factory).TestAll()
}
