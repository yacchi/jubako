package document

import (
	"reflect"
	"testing"
)

// TestDocumentFormatString tests that DocumentFormat constants have correct values.
func TestDocumentFormatString(t *testing.T) {
	tests := []struct {
		format   DocumentFormat
		expected string
	}{
		{FormatYAML, "yaml"},
		{FormatTOML, "toml"},
		{FormatJSONC, "jsonc"},
		{FormatJSON, "json"},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			if string(tt.format) != tt.expected {
				t.Errorf("DocumentFormat = %q, want %q", tt.format, tt.expected)
			}
		})
	}
}

// TestJSONPatchSet_BasicsAndApplyTo tests JSONPatchSet constructors, mutators, and ApplyTo.
func TestJSONPatchSet_BasicsAndApplyTo(t *testing.T) {
	t.Run("constructors", func(t *testing.T) {
		p := NewAddPatch("/a", 1)
		if p.Op != PatchOpAdd || p.Path != "/a" || p.Value != 1 {
			t.Fatalf("NewAddPatch() = %+v", p)
		}
		p = NewRemovePatch("/a")
		if p.Op != PatchOpRemove || p.Path != "/a" {
			t.Fatalf("NewRemovePatch() = %+v", p)
		}
		p = NewReplacePatch("/a", 2)
		if p.Op != PatchOpReplace || p.Path != "/a" || p.Value != 2 {
			t.Fatalf("NewReplacePatch() = %+v", p)
		}
	})

	t.Run("mutators and length helpers", func(t *testing.T) {
		var ps JSONPatchSet
		if !ps.IsEmpty() {
			t.Fatalf("IsEmpty() = false, want true")
		}
		if ps.Len() != 0 {
			t.Fatalf("Len() = %d, want 0", ps.Len())
		}

		ps.Add("/a", 1)
		ps.Replace("/a", 2)
		ps.Remove("/a")
		if ps.IsEmpty() {
			t.Fatalf("IsEmpty() = true, want false")
		}
		if ps.Len() != 3 {
			t.Fatalf("Len() = %d, want 3", ps.Len())
		}
	})

	t.Run("ApplyTo applies add/replace/remove and skips invalid paths", func(t *testing.T) {
		data := map[string]any{
			"a": map[string]any{
				"b": 1,
			},
		}

		ps := JSONPatchSet{
			NewReplacePatch("/a/b", 2),
			NewAddPatch("/a/c", "x"),
			NewRemovePatch("/a/b"),
			{Op: PatchOp("unknown"), Path: "/ignored", Value: 1},
			NewAddPatch("relative/path", 123), // invalid JSON Pointer -> should be skipped
		}

		ps.ApplyTo(data)

		want := map[string]any{
			"a": map[string]any{
				"c": "x",
			},
		}
		if !reflect.DeepEqual(data, want) {
			t.Fatalf("ApplyTo() data = %#v, want %#v", data, want)
		}
	})
}
