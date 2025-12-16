package toml

import (
	"errors"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2/unstable"
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
)

// TestDocument_Apply_CommentPreservation verifies TOML-specific comment preservation.
func TestDocument_Apply_CommentPreservation(t *testing.T) {
	input := []byte("# heading\n[server]\nhost = \"localhost\" # inline\nport = 8080\n")
	doc := New()

	changeset := document.JSONPatchSet{
		document.NewReplacePatch("/server/port", int64(9000)),
	}

	out, err := doc.Apply(input, changeset)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "# heading") {
		t.Error("Apply() did not preserve heading comment")
	}
	if !strings.Contains(s, "host = \"localhost\" # inline") {
		t.Error("Apply() did not preserve inline comment")
	}
	if !strings.Contains(s, "port = 9000") {
		t.Error("Apply() did not include updated value")
	}
}

func TestDocument_MarshalTestData_NullValue(t *testing.T) {
	doc := New()

	testData := map[string]any{
		"key":  "value",
		"null": nil,
	}

	_, err := doc.MarshalTestData(testData)
	if err == nil {
		t.Error("MarshalTestData() should return error for null values")
	}

	var unsupportedErr *document.UnsupportedStructureError
	if !errors.As(err, &unsupportedErr) {
		t.Errorf("MarshalTestData() error type = %T, want *document.UnsupportedStructureError", err)
	}
}

// TestDocument_Compliance runs the standard jktest compliance tests.
func TestDocument_Compliance(t *testing.T) {
	jktest.NewDocumentLayerTester(t, New(),
		jktest.SkipNullTest("TOML format does not support null values"),
	).TestAll()
}

func TestHelper_Basics(t *testing.T) {
	if !equalStringSlice([]string{"a"}, []string{"a"}) {
		t.Fatal("equalStringSlice() = false, want true")
	}
	if equalStringSlice([]string{"a"}, []string{"b"}) {
		t.Fatal("equalStringSlice() = true, want false")
	}
	if equalStringSlice([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("equalStringSlice(different lengths) = true, want false")
	}

	if got := buildPointer([]string{"a", "b/c"}); got != "/a/b~1c" {
		t.Fatalf("buildPointer() = %q", got)
	}
	if got := buildPointer(nil); got != "" {
		t.Fatalf("buildPointer(nil) = %q, want empty string", got)
	}

	if got := firstArrayIndex([]string{"a", "0", "b"}); got != 1 {
		t.Fatalf("firstArrayIndex() = %d, want 1", got)
	}

	if _, err := parseArrayIndex("x"); err == nil {
		t.Fatal("parseArrayIndex() expected error, got nil")
	}
	if n, err := parseArrayIndex("12"); err != nil || n != 12 {
		t.Fatalf("parseArrayIndex() = (%d, %v), want (12, nil)", n, err)
	}
}

func TestContainsNil(t *testing.T) {
	if !containsNil(nil) {
		t.Fatal("containsNil(nil) = false, want true")
	}
	if !containsNil(map[string]any{"a": []any{nil}}) {
		t.Fatal("containsNil(nested nil) = false, want true")
	}
	if containsNil(map[string]any{"a": []any{1}}) {
		t.Fatal("containsNil(no nil) = true, want false")
	}
}

func TestCheckNilMapAndSlice(t *testing.T) {
	if err := checkNilMap("", map[string]any{"a": nil}); err == nil {
		t.Fatal("checkNilMap() expected error, got nil")
	}
	if err := checkNilMap("", map[string]any{"a": map[string]any{"b": nil}}); err == nil {
		t.Fatal("checkNilMap(nested map) expected error, got nil")
	}
	if err := checkNilMap("", map[string]any{"a": []any{nil}}); err == nil {
		t.Fatal("checkNilMap(nested slice) expected error, got nil")
	}
	if err := checkNilSlice("/a", []any{nil}); err == nil {
		t.Fatal("checkNilSlice() expected error, got nil")
	}
	if err := checkNilSlice("/a", []any{[]any{nil}}); err == nil {
		t.Fatal("checkNilSlice(nested slice) expected error, got nil")
	}
}

func TestFormatTOMLValue_Errors(t *testing.T) {
	if _, err := formatTOMLValue(nil); err == nil {
		t.Fatal("formatTOMLValue(nil) expected error, got nil")
	}
	_, err := formatTOMLValue(func() {})
	if err == nil {
		t.Fatal("formatTOMLValue(func) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to encode TOML value") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestFormatTOMLValue_MarshalWeirdOutput(t *testing.T) {
	orig := tomlMarshal
	t.Cleanup(func() { tomlMarshal = orig })

	tomlMarshal = func(any) ([]byte, error) { return []byte("no_equal"), nil }
	if _, err := formatTOMLValue(1); err == nil {
		t.Fatal("formatTOMLValue() expected error, got nil")
	}

	tomlMarshal = func(any) ([]byte, error) { return []byte("x = 1"), nil } // no newline
	if got, err := formatTOMLValue(1); err != nil || got != "1" {
		t.Fatalf("formatTOMLValue() = (%q, %v), want (\"1\", nil)", got, err)
	}
}

func TestBuildIndex_InvalidTOML(t *testing.T) {
	_, err := buildIndex([]byte("[broken"))
	if err == nil {
		t.Fatal("buildIndex() expected error, got nil")
	}
}

func TestDocument_Get_InvalidTOML(t *testing.T) {
	doc := New()
	_, err := doc.Get([]byte("a = "))
	if err == nil {
		t.Fatal("Get() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse TOML") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestDocument_Get_CommentOnly(t *testing.T) {
	doc := New()
	got, err := doc.Get([]byte("# comment\n"))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("Get(comment) = %#v, want empty map", got)
	}
}

func TestDocument_MarshalTestData_MarshalError(t *testing.T) {
	doc := New()
	_, err := doc.MarshalTestData(map[string]any{"ch": make(chan int)})
	if err == nil {
		t.Fatal("MarshalTestData() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to marshal TOML test data") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestDocument_Apply_UnmarshalError(t *testing.T) {
	doc := New()
	orig := tomlUnmarshal
	tomlUnmarshal = func([]byte, any) error { return errors.New("unmarshal error") }
	t.Cleanup(func() { tomlUnmarshal = orig })

	_, err := doc.Apply([]byte("a = 1\n"), document.JSONPatchSet{document.NewReplacePatch("/a", 2)})
	if err == nil {
		t.Fatal("Apply() expected error, got nil")
	}
}

func TestDocument_Apply_CheckNilError(t *testing.T) {
	doc := New()
	orig := tomlUnmarshal
	tomlUnmarshal = func(_ []byte, v any) error {
		if m, ok := v.(*map[string]any); ok {
			*m = map[string]any{"a": nil}
		}
		return nil
	}
	t.Cleanup(func() { tomlUnmarshal = orig })

	_, err := doc.Apply([]byte("a = 1\n"), nil)
	if err == nil {
		t.Fatal("Apply() expected error, got nil")
	}
	var us *document.UnsupportedStructureError
	if !errors.As(err, &us) {
		t.Fatalf("expected UnsupportedStructureError, got %T", err)
	}
}

func TestDocument_Apply_EmptyChangeset(t *testing.T) {
	doc := New()
	out, err := doc.Apply([]byte("a = 1\n"), document.JSONPatchSet{})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := doc.Get(out); err != nil {
		t.Fatalf("Get(Apply()) error = %v, output=%q", err, string(out))
	}
}

func TestDocument_Apply_EmptyInputWithChangeset(t *testing.T) {
	doc := New()
	out, err := doc.Apply(nil, document.JSONPatchSet{
		document.NewAddPatch("", 1),                      // root path -> skipped
		{Op: document.PatchOpAdd, Path: "rel", Value: 1}, // invalid path -> skipped
		document.NewAddPatch("/a", 1),
		document.NewAddPatch("/b", map[string]any{"c": 2}),
		document.NewAddPatch("/skip", nil),
		document.NewRemovePatch("/missing"),
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !strings.Contains(string(out), "a") || !strings.Contains(string(out), "b") {
		t.Fatalf("unexpected output: %q", string(out))
	}
	if _, err := doc.Get(out); err != nil {
		t.Fatalf("Get(output) error = %v, output=%q", err, string(out))
	}
}

func TestDocument_Apply_NonEmptyInput_SkipsRootAndContinuesAfterErrors(t *testing.T) {
	doc := New()
	out, err := doc.Apply([]byte("a = 1\n"), document.JSONPatchSet{
		{Op: document.PatchOpAdd, Path: "", Value: map[string]any{"x": 1}}, // root path -> skipped
		{Op: document.PatchOpAdd, Path: "rel", Value: 1},                   // invalid path -> skipped
		{Op: document.PatchOpAdd, Path: "/0/a", Value: 1},                  // invalid array path -> ignored
		document.NewRemovePatch("/0"),                                      // invalid array path -> ignored
		document.NewReplacePatch("/a", 2),                                  // applied
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !strings.Contains(string(out), "a = 2") {
		t.Fatalf("unexpected output: %q", string(out))
	}
	if _, err := doc.Get(out); err != nil {
		t.Fatalf("Get(output) error = %v, output=%q", err, string(out))
	}
}

func TestApplySetNonIndexed_ReplaceKeepsValidTOML(t *testing.T) {
	src := []byte("existing = \"value\"\n")
	out, err := applySetNonIndexed(src, []string{"existing"}, "replaced")
	if err != nil {
		t.Fatalf("applySetNonIndexed() error = %v", err)
	}
	doc := New()
	if _, err := doc.Get(out); err != nil {
		t.Fatalf("Get(output) error = %v, output=%q", err, string(out))
	}
}

func TestApplySetNonIndexed_ValueErrorAndInvalidTOML(t *testing.T) {
	if _, err := applySetNonIndexed([]byte("a = 1\n"), []string{"a"}, nil); err == nil {
		t.Fatal("applySetNonIndexed(nil value) expected error, got nil")
	}
	if _, err := applySetNonIndexed([]byte("[broken"), []string{"a"}, 1); err == nil {
		t.Fatal("applySetNonIndexed(invalid TOML) expected error, got nil")
	}
}

func TestApplyDeleteNonIndexed_BuildIndexErrorAndNotFound(t *testing.T) {
	if _, err := applyDeleteNonIndexed([]byte("[broken"), []string{"a"}); err == nil {
		t.Fatal("applyDeleteNonIndexed(invalid TOML) expected error, got nil")
	}
	src := []byte("a = 1\n")
	out, err := applyDeleteNonIndexed(src, []string{"missing"})
	if err != nil {
		t.Fatalf("applyDeleteNonIndexed() error = %v", err)
	}
	if string(out) != string(src) {
		t.Fatalf("applyDeleteNonIndexed(not found) = %q, want %q", string(out), string(src))
	}
}

func TestApplyHelpers_ErrorBranches(t *testing.T) {
	if _, err := applySetNonIndexed(nil, nil, 1); err == nil {
		t.Fatal("applySetNonIndexed(empty keys) expected error, got nil")
	}
	if _, err := applyDeleteNonIndexed(nil, nil); err == nil {
		t.Fatal("applyDeleteNonIndexed(empty keys) expected error, got nil")
	}

	// Cover no-index fast paths.
	if out, err := applySetWithArray(nil, []string{"a"}, 1); err != nil || !strings.Contains(string(out), "a") {
		t.Fatalf("applySetWithArray(no index) = (%q, %v)", string(out), err)
	}
	if out, err := applyDeleteWithArray([]byte("a = 1\n"), []string{"a"}); err != nil || strings.Contains(string(out), "a = 1") {
		t.Fatalf("applyDeleteWithArray(no index) = (%q, %v)", string(out), err)
	}

	// Cover src-empty branches for array operations.
	if out, err := applySetWithArray(nil, []string{"arr", "0"}, 1); err != nil || !strings.Contains(string(out), "arr") {
		t.Fatalf("applySetWithArray(empty src) = (%q, %v)", string(out), err)
	}
	if out, err := applyDeleteWithArray(nil, []string{"arr", "0"}); err != nil || len(out) != 0 {
		t.Fatalf("applyDeleteWithArray(empty src) = (%q, %v)", string(out), err)
	}
}

func TestBuildIndex_ArrayTable(t *testing.T) {
	_, err := buildIndex([]byte("[[servers]]\nname = \"a\"\n[[servers]]\nname = \"b\"\n"))
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
}

func TestEnsureSectionEnd(t *testing.T) {
	src := []byte("[a]\nx = 1\n")
	idx, err := buildIndex(src)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}

	pos := idx.ensureSectionEnd(nil, &src)
	if pos != 0 {
		t.Fatalf("ensureSectionEnd(root) = %d, want 0", pos)
	}

	pos = idx.ensureSectionEnd([]string{"a"}, &src)
	if pos <= 0 {
		t.Fatalf("ensureSectionEnd(existing) = %d", pos)
	}

	src2 := []byte("x = 1")
	idx2, err := buildIndex(src2)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	_ = idx2.ensureSectionEnd([]string{"b"}, &src2)
	if !strings.Contains(string(src2), "[b]") {
		t.Fatalf("expected header insertion, got %q", string(src2))
	}
}

func TestReplaceAndInsertBytes_Bounds(t *testing.T) {
	if got := string(replaceBytes([]byte("abc"), -1, 1, []byte("x"))); got != "xbc" {
		t.Fatalf("replaceBytes() = %q", got)
	}
	if got := string(replaceBytes([]byte("abc"), 2, 1, []byte("x"))); got != "abxc" {
		t.Fatalf("replaceBytes(end<start) = %q", got)
	}
	if got := string(insertBytes([]byte("abc"), -1, []byte("x"))); got != "xabc" {
		t.Fatalf("insertBytes(neg) = %q", got)
	}
	if got := string(insertBytes([]byte("abc"), 9, []byte("x"))); got != "abcx" {
		t.Fatalf("insertBytes(too large) = %q", got)
	}
}

func TestEnsureLeadingNewline(t *testing.T) {
	line := []byte("x\n")
	if got := string(ensureLeadingNewline([]byte(""), 0, line)); got != "x\n" {
		t.Fatalf("pos=0 = %q", got)
	}
	if got := string(ensureLeadingNewline([]byte("a"), 1, line)); got != "\nx\n" {
		t.Fatalf("no newline before insert = %q", got)
	}
	if got := string(ensureLeadingNewline([]byte("a\n"), 2, line)); got != "x\n" {
		t.Fatalf("newline already present = %q", got)
	}
}

func TestFormatKey_QuotingAndEscaping(t *testing.T) {
	if got := formatKey("simple_key"); got != "simple_key" {
		t.Fatalf("formatKey(bare) = %q", got)
	}
	if got := formatKey("has space"); got == "has space" || !strings.Contains(got, "has space") {
		t.Fatalf("formatKey(space) = %q", got)
	}
	if got := formatKey(`has"quote`); got == `has"quote` || !strings.Contains(got, `has"quote`) {
		t.Fatalf("formatKey(quote) = %q", got)
	}

	// Force formatTOMLValue to fail to cover the fallback quoting branch.
	orig := tomlMarshal
	tomlMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal error") }
	t.Cleanup(func() { tomlMarshal = orig })

	got := formatKey(`has"quote`)
	if !strings.HasPrefix(got, "\"") {
		t.Fatalf("formatKey(fallback) = %q", got)
	}
}

func TestCheckNilSlice_RecursesIntoMapAndSlice(t *testing.T) {
	err := checkNilSlice("/a", []any{map[string]any{"b": []any{nil}}})
	if err == nil {
		t.Fatal("checkNilSlice() expected error, got nil")
	}
}

func TestApplySetWithArray_ErrorsAndUpdates(t *testing.T) {
	t.Run("path cannot start with array index", func(t *testing.T) {
		_, err := applySetWithArray([]byte(""), []string{"0", "a"}, 1)
		var ip *document.InvalidPathError
		if !errors.As(err, &ip) {
			t.Fatalf("expected InvalidPathError, got %T", err)
		}
	})

	t.Run("type mismatch for container", func(t *testing.T) {
		_, err := applySetWithArray([]byte("a = 1\n"), []string{"a", "0"}, 1)
		var tm *document.TypeMismatchError
		if !errors.As(err, &tm) {
			t.Fatalf("expected TypeMismatchError, got %T", err)
		}
	})

	t.Run("arrays cannot be extended with gaps", func(t *testing.T) {
		_, err := applySetWithArray([]byte("arr = []\n"), []string{"arr", "2"}, 1)
		var us *document.UnsupportedStructureError
		if !errors.As(err, &us) {
			t.Fatalf("expected UnsupportedStructureError, got %T", err)
		}
	})

	t.Run("nested array and object updates", func(t *testing.T) {
		src := []byte("arr = []\n")
		out, err := applySetWithArray(src, []string{"arr", "0", "0"}, 1)
		if err != nil {
			t.Fatalf("applySetWithArray() error = %v", err)
		}
		s := string(out)
		if !strings.Contains(s, "arr") {
			t.Fatalf("unexpected output: %q", s)
		}
	})
}

func TestApplySetApplyDelete_IndexDetection(t *testing.T) {
	src := []byte("arr = []\n")
	out, err := applySet(src, []string{"arr", "0"}, 1)
	if err != nil || !strings.Contains(string(out), "arr") {
		t.Fatalf("applySet() = (%q, %v)", string(out), err)
	}

	out, err = applyDelete([]byte("arr = [1]\n"), []string{"arr", "0"})
	if err != nil {
		t.Fatalf("applyDelete() error = %v", err)
	}
}

func TestApplySetWithArray_UnmarshalError(t *testing.T) {
	orig := tomlUnmarshal
	tomlUnmarshal = func([]byte, any) error { return errors.New("unmarshal error") }
	t.Cleanup(func() { tomlUnmarshal = orig })

	if _, err := applySetWithArray([]byte("arr = []\n"), []string{"arr", "0"}, 1); err == nil {
		t.Fatal("applySetWithArray() expected error, got nil")
	}
	if _, err := applyDeleteWithArray([]byte("arr = []\n"), []string{"arr", "0"}); err == nil {
		t.Fatal("applyDeleteWithArray() expected error, got nil")
	}
}

func TestApplyDeleteWithArray(t *testing.T) {
	t.Run("path cannot start with array index", func(t *testing.T) {
		_, err := applyDeleteWithArray([]byte(""), []string{"0", "a"})
		var ip *document.InvalidPathError
		if !errors.As(err, &ip) {
			t.Fatalf("expected InvalidPathError, got %T", err)
		}
	})

	t.Run("type mismatch for container", func(t *testing.T) {
		_, err := applyDeleteWithArray([]byte("a = 1\n"), []string{"a", "0"})
		var tm *document.TypeMismatchError
		if !errors.As(err, &tm) {
			t.Fatalf("expected TypeMismatchError, got %T", err)
		}
	})

	t.Run("delete element", func(t *testing.T) {
		src := []byte("arr = [1, 2]\n")
		out, err := applyDeleteWithArray(src, []string{"arr", "0"})
		if err != nil {
			t.Fatalf("applyDeleteWithArray() error = %v", err)
		}
		if strings.Contains(string(out), "1, 2") {
			t.Fatalf("unexpected output: %q", string(out))
		}
	})
}

func TestApplyDeleteWithArray_MissingContainerIsNoop(t *testing.T) {
	src := []byte("a = 1\n")
	out, err := applyDeleteWithArray(src, []string{"arr", "0"})
	if err != nil {
		t.Fatalf("applyDeleteWithArray() error = %v", err)
	}
	if string(out) != string(src) {
		t.Fatalf("output = %q, want %q", string(out), string(src))
	}
}

func TestArrayHelpers(t *testing.T) {
	if _, err := setAnyInArray([]any{}, nil, 1); err == nil {
		t.Fatal("setAnyInArray(empty keys) expected error, got nil")
	}

	arr, err := setAnyInArray([]any{}, []string{"0"}, 1)
	if err != nil {
		t.Fatalf("setAnyInArray() error = %v", err)
	}
	if len(arr) != 1 || arr[0] != 1 {
		t.Fatalf("setAnyInArray() = %#v", arr)
	}

	_, err = setAnyInArray([]any{}, []string{"x"}, 1)
	var ip *document.InvalidPathError
	if !errors.As(err, &ip) {
		t.Fatalf("expected InvalidPathError, got %T", err)
	}

	_, err = setAnyInArray([]any{map[string]any{}}, []string{"0", "0"}, 1)
	var tm *document.TypeMismatchError
	if !errors.As(err, &tm) {
		t.Fatalf("expected TypeMismatchError, got %T", err)
	}

	_, err = setAnyInArray([]any{[]any{}}, []string{"0", "1"}, 1)
	var us *document.UnsupportedStructureError
	if !errors.As(err, &us) {
		t.Fatalf("expected UnsupportedStructureError, got %T", err)
	}

	arr, err = setAnyInArray([]any{nil}, []string{"0", "a"}, 1)
	if err != nil {
		t.Fatalf("setAnyInArray(elem nil) error = %v", err)
	}
	if m, ok := arr[0].(map[string]any); !ok || m["a"] != 1 {
		t.Fatalf("setAnyInArray(elem nil) = %#v", arr)
	}

	arr, err = setAnyInArray([]any{[]any{}}, []string{"0", "0"}, 1)
	if err != nil {
		t.Fatalf("setAnyInArray(nested) error = %v", err)
	}
	if nested := arr[0].([]any); len(nested) != 1 || nested[0] != 1 {
		t.Fatalf("setAnyInArray(nested) = %#v", arr)
	}

	arr, err = setAnyInArray([]any{map[string]any{}}, []string{"0", "x"}, 1)
	if err != nil {
		t.Fatalf("setAnyInArray(object) error = %v", err)
	}

	_, err = setAnyInArray([]any{1}, []string{"0", "x"}, 1)
	if !errors.As(err, &tm) {
		t.Fatalf("expected TypeMismatchError, got %T", err)
	}

	_, err = setAnyInArray([]any{}, []string{"2"}, 1)
	if !errors.As(err, &us) {
		t.Fatalf("expected UnsupportedStructureError, got %T", err)
	}

	if _, err := deleteAnyInArray([]any{}, nil); err == nil {
		t.Fatal("deleteAnyInArray(empty keys) expected error, got nil")
	}
	if _, err := deleteAnyInArray([]any{1}, []string{"x"}); err == nil {
		t.Fatal("deleteAnyInArray(invalid index) expected error, got nil")
	}

	got, err := deleteAnyInArray([]any{1, 2}, []string{"0"})
	if err != nil {
		t.Fatalf("deleteAnyInArray() error = %v", err)
	}
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("deleteAnyInArray() = %#v", got)
	}

	got, err = deleteAnyInArray([]any{[]any{1, 2}}, []string{"0", "0"})
	if err != nil {
		t.Fatalf("deleteAnyInArray(nested) error = %v", err)
	}
	if nested := got[0].([]any); len(nested) != 1 || nested[0] != 2 {
		t.Fatalf("deleteAnyInArray(nested) = %#v", got)
	}

	got, err = deleteAnyInArray([]any{map[string]any{"a": 1}}, []string{"0", "a"})
	if err != nil {
		t.Fatalf("deleteAnyInArray(object) error = %v", err)
	}
	if v := got[0].(map[string]any); len(v) != 0 {
		t.Fatalf("deleteAnyInArray(object) = %#v", got)
	}

	got, err = deleteAnyInArray([]any{"x"}, []string{"0", "0"})
	if err != nil {
		t.Fatalf("deleteAnyInArray(non-container) error = %v", err)
	}

	got, err = deleteAnyInArray([]any{1}, []string{"9"})
	if err != nil || len(got) != 1 {
		t.Fatalf("deleteAnyInArray(out of range) = (%#v, %v)", got, err)
	}

	got, err = deleteAnyInArray([]any{1}, []string{"0", "a"})
	if err != nil {
		t.Fatalf("deleteAnyInArray(object mismatch) error = %v", err)
	}
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("deleteAnyInArray(object mismatch) = %#v", got)
	}
}

func TestGetAnySetAny(t *testing.T) {
	root := map[string]any{}
	setAny(root, []string{"a", "b"}, 1)
	v, ok := getAny(root, []string{"a", "b"})
	if !ok || v != 1 {
		t.Fatalf("getAny() = (%v, %v), want (1, true)", v, ok)
	}
}

func TestDocument_Apply_SkipCases(t *testing.T) {
	doc := New()

	out, err := doc.Apply([]byte("a = 1\n"), document.JSONPatchSet{
		{Op: document.PatchOpAdd, Path: "relative", Value: 1}, // invalid path -> skipped
		{Op: document.PatchOpAdd, Path: "/", Value: 1},        // root keys == 1? ("/" becomes [""]) -> not empty but leaf is empty key
		{Op: document.PatchOpReplace, Path: "/a", Value: nil}, // nil value -> skipped
		{Op: document.PatchOpRemove, Path: "relative"},        // invalid path -> skipped
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !strings.Contains(string(out), "a = 1") {
		t.Fatalf("unexpected output: %q", string(out))
	}
}

func TestBuildIndex_KeyValueWithoutValueTriggersKeyValueInfoError(t *testing.T) {
	_, err := buildIndex([]byte("a =\n"))
	if err == nil {
		t.Fatal("buildIndex() expected error, got nil")
	}
}

func TestRangeOffset_DataFallbackAndZero(t *testing.T) {
	src := []byte("abc")
	p := &unstable.Parser{}
	p.Reset(src)

	if off := rangeOffset(p, &unstable.Node{}); off != 0 {
		t.Fatalf("rangeOffset(empty) = %d, want 0", off)
	}

	n := &unstable.Node{Data: src[1:2]} // subslice of parser input
	if off := rangeOffset(p, n); off != 1 {
		t.Fatalf("rangeOffset(Data) = %d, want 1", off)
	}
}

func TestFindLineStart_NoNewlineBeforeOffset(t *testing.T) {
	if got := findLineStart([]byte("abc"), 2); got != 0 {
		t.Fatalf("findLineStart(no newline) = %d, want 0", got)
	}
}

func TestBuildIndex_SectionEndComputation(t *testing.T) {
	src := []byte("[a]\nx = 1\n[b]\ny = 2\n")
	idx, err := buildIndex(src)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if len(idx.sections) < 2 {
		t.Fatalf("sections = %d, want >= 2", len(idx.sections))
	}
	if idx.sections[0].lineEnd != idx.sections[1].lineStart {
		t.Fatalf("section end = %d, want %d", idx.sections[0].lineEnd, idx.sections[1].lineStart)
	}
}
