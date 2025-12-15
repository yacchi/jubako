package decoder

import (
	"strings"
	"testing"
)

type jsonTarget struct {
	A string `json:"a"`
}

func TestJSON(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var dst jsonTarget
		if err := JSON(map[string]any{"a": "ok"}, &dst); err != nil {
			t.Fatalf("JSON() error = %v", err)
		}
		if dst.A != "ok" {
			t.Fatalf("decoded value = %q, want %q", dst.A, "ok")
		}
	})

	t.Run("marshal error", func(t *testing.T) {
		var dst jsonTarget
		err := JSON(map[string]any{"a": func() {}}, &dst)
		if err == nil {
			t.Fatal("JSON() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to marshal map") {
			t.Fatalf("error = %q, want to contain %q", err.Error(), "failed to marshal map")
		}
	})

	t.Run("unmarshal error", func(t *testing.T) {
		var dst jsonTarget
		err := JSON(map[string]any{"a": "ok"}, dst) // non-pointer target
		if err == nil {
			t.Fatal("JSON() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to unmarshal to target type") {
			t.Fatalf("error = %q, want to contain %q", err.Error(), "failed to unmarshal to target type")
		}
	})
}
