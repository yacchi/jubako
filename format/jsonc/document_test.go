package jsonc

import (
	"strings"
	"testing"

	"github.com/tailscale/hujson"
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
)

// TestDocument_Apply_CommentPreservation verifies JSONC-specific comment preservation.
func TestDocument_Apply_CommentPreservation(t *testing.T) {
	input := []byte(`{
  // heading
  "server": {
    "host": "localhost" // inline
  }
}
`)
	doc := New()

	changeset := document.JSONPatchSet{
		document.NewAddPatch("/server/port", 9000),
	}

	out, err := doc.Apply(input, changeset)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "// heading") {
		t.Error("Apply() did not preserve heading comment")
	}
	if !strings.Contains(s, "inline") {
		t.Error("Apply() did not preserve inline comment")
	}
	if !strings.Contains(s, "port") || !strings.Contains(s, "9000") {
		t.Error("Apply() did not include updated value")
	}
}

// TestDocument_Compliance runs the standard jktest compliance tests.
func TestDocument_Compliance(t *testing.T) {
	jktest.NewDocumentLayerTester(t, New(),
		jktest.SkipSaveArrayTest("hujson Patch API does not support array append at index == len"),
	).TestAll()
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

func TestGet_NullReturnsEmptyMap(t *testing.T) {
	doc := New()
	got, err := doc.Get([]byte("null"))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("Get(null) = %#v, want empty map", got)
	}
}

func TestGet_NonObjectFailsDecode(t *testing.T) {
	doc := New()
	_, err := doc.Get([]byte("[]"))
	if err == nil {
		t.Fatal("Get([]) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to decode JSONC") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestGet_Invalid(t *testing.T) {
	doc := New()
	_, err := doc.Get([]byte("{ invalid"))
	if err == nil {
		t.Fatal("Get() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse JSONC") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestApply_NoChangeset(t *testing.T) {
	doc := New()

	out, err := doc.Apply(nil, nil)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if string(out) != "{}\n" {
		t.Fatalf("Apply(nil,nil) = %q, want %q", string(out), "{}\\n")
	}

	_, err = doc.Apply([]byte("{ invalid"), nil)
	if err == nil {
		t.Fatal("Apply(invalid,nil) expected error, got nil")
	}
}

func TestApply_NoChangeset_ValidInputReturnsPack(t *testing.T) {
	doc := New()
	out, err := doc.Apply([]byte("{\n // c\n \"a\": 1\n}\n"), nil)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !strings.Contains(string(out), "\"a\"") {
		t.Fatalf("unexpected output: %q", string(out))
	}
}

func TestApply_Changeset_ParseFallbackAndSkips(t *testing.T) {
	doc := New()

	out, err := doc.Apply([]byte("{ invalid"), document.JSONPatchSet{
		document.NewAddPatch("/a/b", 1),
		{Op: document.PatchOpAdd, Path: "/bad", Value: func() {}},     // marshal error -> skipped
		{Op: document.PatchOpAdd, Path: "relative", Value: 1},         // patch error -> skipped
		{Op: document.PatchOp("unknown"), Path: "/ignored", Value: 1}, // op skipped
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !strings.Contains(string(out), "\"a\"") {
		t.Fatalf("output did not include applied patch: %q", string(out))
	}
}

func TestApply_Changeset_EmptyInput(t *testing.T) {
	doc := New()
	out, err := doc.Apply(nil, document.JSONPatchSet{
		document.NewAddPatch("/a", 1),
		document.NewReplacePatch("/a", 2),
		document.NewRemovePatch("/a"),
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if string(out) != "{}" {
		t.Fatalf("Apply() = %q, want %q", string(out), "{}")
	}
}

func TestApply_Changeset_EnsureIntermediateObjectsErrorSkips(t *testing.T) {
	doc := New()
	out, err := doc.Apply([]byte("1"), document.JSONPatchSet{
		document.NewAddPatch("/a/b", 1),
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if string(out) != "1" {
		t.Fatalf("Apply() = %q, want %q", string(out), "1")
	}
}

func TestMarshalTestData_Error(t *testing.T) {
	doc := New()
	_, err := doc.MarshalTestData(map[string]any{"x": func() {}})
	if err == nil {
		t.Fatal("MarshalTestData() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to marshal JSONC test data") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestEnsureIntermediateObjects(t *testing.T) {
	t.Run("no-op paths", func(t *testing.T) {
		v, _ := hujson.Parse([]byte("{}"))
		if err := ensureIntermediateObjects(&v, ""); err != nil {
			t.Fatalf("ensureIntermediateObjects(\"\") error = %v", err)
		}
		if err := ensureIntermediateObjects(&v, "/"); err != nil {
			t.Fatalf("ensureIntermediateObjects(\"/\") error = %v", err)
		}
		if err := ensureIntermediateObjects(&v, "/a"); err != nil {
			t.Fatalf("ensureIntermediateObjects(\"/a\") error = %v", err)
		}
	})

	t.Run("creates missing objects for nested add", func(t *testing.T) {
		v, _ := hujson.Parse([]byte("{}"))
		if err := ensureIntermediateObjects(&v, "/a/b"); err != nil {
			t.Fatalf("ensureIntermediateObjects() error = %v", err)
		}
		if !strings.Contains(string(v.Pack()), "\"a\"") {
			t.Fatalf("expected intermediate object creation, got %q", string(v.Pack()))
		}
	})

	t.Run("existing object continues deeper", func(t *testing.T) {
		v, _ := hujson.Parse([]byte("{\"a\":{}}"))
		if err := ensureIntermediateObjects(&v, "/a/b/c"); err != nil {
			t.Fatalf("ensureIntermediateObjects() error = %v", err)
		}
		out := string(v.Pack())
		if !strings.Contains(out, "\"b\"") {
			t.Fatalf("expected intermediate creation under existing object, got %q", out)
		}
	})

	t.Run("root is array triggers map unmarshal failure path", func(t *testing.T) {
		v, _ := hujson.Parse([]byte("[]"))
		if err := ensureIntermediateObjects(&v, "/a/b"); err == nil {
			t.Fatal("ensureIntermediateObjects() expected error, got nil")
		}
		if string(v.Pack()) != "[]" {
			t.Fatalf("unexpected modification: %q", string(v.Pack()))
		}
	})

	t.Run("existing non-object stops creation", func(t *testing.T) {
		v, _ := hujson.Parse([]byte("{\"a\":1}"))
		if err := ensureIntermediateObjects(&v, "/a/b/c"); err == nil {
			t.Fatal("ensureIntermediateObjects() expected error, got nil")
		}
		if string(v.Pack()) != "{\"a\":1}" {
			t.Fatalf("unexpected modification: %q", string(v.Pack()))
		}
	})

	t.Run("invalid root bytes returns without error", func(t *testing.T) {
		var v hujson.Value
		if err := ensureIntermediateObjects(&v, "/a/b"); err == nil {
			t.Fatal("ensureIntermediateObjects() expected error, got nil")
		}
	})

	t.Run("scalar root hits unmarshal-failure path", func(t *testing.T) {
		v, _ := hujson.Parse([]byte("1"))
		if err := ensureIntermediateObjects(&v, "/a/b"); err == nil {
			t.Fatal("ensureIntermediateObjects() expected error, got nil")
		}
	})
}
