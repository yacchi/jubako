package jsonptr

import (
	"reflect"
	"testing"
)

func TestEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special characters",
			input: "simple",
			want:  "simple",
		},
		{
			name:  "tilde",
			input: "~",
			want:  "~0",
		},
		{
			name:  "slash",
			input: "/",
			want:  "~1",
		},
		{
			name:  "both tilde and slash",
			input: "~/",
			want:  "~0~1",
		},
		{
			name:  "multiple tildes",
			input: "~~~",
			want:  "~0~0~0",
		},
		{
			name:  "multiple slashes",
			input: "///",
			want:  "~1~1~1",
		},
		{
			name:  "real path example",
			input: "/api/users",
			want:  "~1api~1users",
		},
		{
			name:  "feature flag example",
			input: "enable/disable",
			want:  "enable~1disable",
		},
		{
			name:  "tilde then slash",
			input: "~foo/bar",
			want:  "~0foo~1bar",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Escape(tt.input)
			if got != tt.want {
				t.Errorf("Escape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestUnescape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special characters",
			input: "simple",
			want:  "simple",
		},
		{
			name:  "escaped tilde",
			input: "~0",
			want:  "~",
		},
		{
			name:  "escaped slash",
			input: "~1",
			want:  "/",
		},
		{
			name:  "both escaped",
			input: "~0~1",
			want:  "~/",
		},
		{
			name:  "multiple escaped tildes",
			input: "~0~0~0",
			want:  "~~~",
		},
		{
			name:  "multiple escaped slashes",
			input: "~1~1~1",
			want:  "///",
		},
		{
			name:  "real path example",
			input: "~1api~1users",
			want:  "/api/users",
		},
		{
			name:  "feature flag example",
			input: "enable~1disable",
			want:  "enable/disable",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unescape(tt.input)
			if got != tt.want {
				t.Errorf("Unescape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeUnescape_RoundTrip(t *testing.T) {
	tests := []string{
		"simple",
		"~",
		"/",
		"~/",
		"/api/users",
		"enable/disable",
		"~foo/bar",
		"~~~",
		"///",
		"",
		"feature~flags/enable~disable",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			escaped := Escape(input)
			unescaped := Unescape(escaped)
			if unescaped != input {
				t.Errorf("Round trip failed: %q -> %q -> %q", input, escaped, unescaped)
			}
		})
	}
}

func TestBuild(t *testing.T) {
	tests := []struct {
		name string
		keys []any
		want string
	}{
		{
			name: "empty",
			keys: []any{},
			want: "",
		},
		{
			name: "single string key",
			keys: []any{"server"},
			want: "/server",
		},
		{
			name: "multiple string keys",
			keys: []any{"server", "port"},
			want: "/server/port",
		},
		{
			name: "array index",
			keys: []any{"servers", 0, "name"},
			want: "/servers/0/name",
		},
		{
			name: "with special characters",
			keys: []any{"feature.flags", "enable/disable"},
			want: "/feature.flags/enable~1disable",
		},
		{
			name: "with path in key",
			keys: []any{"paths", "/api/users"},
			want: "/paths/~1api~1users",
		},
		{
			name: "with tilde",
			keys: []any{"~foo", "bar"},
			want: "/~0foo/bar",
		},
		{
			name: "int types",
			keys: []any{0, 1, 2},
			want: "/0/1/2",
		},
		{
			name: "int64",
			keys: []any{int64(123), "value"},
			want: "/123/value",
		},
		{
			name: "uint",
			keys: []any{uint(456), "value"},
			want: "/456/value",
		},
		{
			name: "uint64",
			keys: []any{uint64(789), "value"},
			want: "/789/value",
		},
		{
			name: "empty string key",
			keys: []any{""},
			want: "/",
		},
		{
			name: "root then empty",
			keys: []any{"root", ""},
			want: "/root/",
		},
		{
			name: "default fmt.Sprint branch",
			keys: []any{true, "value"},
			want: "/true/value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Build(tt.keys...)
			if got != tt.want {
				t.Errorf("Build(%v) = %q, want %q", tt.keys, got, tt.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		pointer string
		want    []string
		wantErr bool
	}{
		{
			name:    "empty pointer",
			pointer: "",
			want:    []string{},
			wantErr: false,
		},
		{
			name:    "root only",
			pointer: "/",
			want:    []string{""},
			wantErr: false,
		},
		{
			name:    "single key",
			pointer: "/server",
			want:    []string{"server"},
			wantErr: false,
		},
		{
			name:    "multiple keys",
			pointer: "/server/port",
			want:    []string{"server", "port"},
			wantErr: false,
		},
		{
			name:    "array index",
			pointer: "/servers/0/name",
			want:    []string{"servers", "0", "name"},
			wantErr: false,
		},
		{
			name:    "with escaped slash",
			pointer: "/feature.flags/enable~1disable",
			want:    []string{"feature.flags", "enable/disable"},
			wantErr: false,
		},
		{
			name:    "with escaped tilde",
			pointer: "/~0foo/bar",
			want:    []string{"~foo", "bar"},
			wantErr: false,
		},
		{
			name:    "with both escapes",
			pointer: "/~0~1/test",
			want:    []string{"~/", "test"},
			wantErr: false,
		},
		{
			name:    "invalid: no leading slash",
			pointer: "server/port",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid: starts with word",
			pointer: "server",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty key",
			pointer: "/root//child",
			want:    []string{"root", "", "child"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.pointer)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse(%q) error = %v, wantErr %v", tt.pointer, err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) = %v, want %v", tt.pointer, got, tt.want)
			}
		})
	}
}

func TestBuildParse_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		keys []any
	}{
		{
			name: "simple path",
			keys: []any{"server", "port"},
		},
		{
			name: "with array index",
			keys: []any{"servers", 0, "name"},
		},
		{
			name: "with special characters",
			keys: []any{"feature.flags", "enable/disable"},
		},
		{
			name: "with tilde and slash",
			keys: []any{"~foo", "bar/baz"},
		},
		{
			name: "empty key",
			keys: []any{"root", "", "child"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pointer := Build(tt.keys...)
			parsed, err := Parse(pointer)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			// Convert keys to strings for comparison
			wantKeys := make([]string, len(tt.keys))
			for i, key := range tt.keys {
				switch v := key.(type) {
				case string:
					wantKeys[i] = v
				case int:
					wantKeys[i] = string(rune('0' + v))
				}
			}

			if !reflect.DeepEqual(parsed, wantKeys) {
				t.Errorf("Round trip failed: %v -> %q -> %v", tt.keys, pointer, parsed)
			}
		})
	}
}

func TestParseBuild_RoundTrip(t *testing.T) {
	tests := []string{
		"/server",
		"/server/port",
		"/servers/0/name",
		"/feature.flags/enable~1disable",
		"/~0foo/bar",
		"/~0~1/test",
		"/",
		"/root//child",
	}

	for _, pointer := range tests {
		t.Run(pointer, func(t *testing.T) {
			parsed, err := Parse(pointer)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			// Convert parsed keys to []any for Build
			keys := make([]any, len(parsed))
			for i, k := range parsed {
				keys[i] = k
			}

			rebuilt := Build(keys...)
			if rebuilt != pointer {
				t.Errorf("Round trip failed: %q -> %v -> %q", pointer, parsed, rebuilt)
			}
		})
	}
}

func BenchmarkEscape(b *testing.B) {
	input := "/api/v1/users/~username"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Escape(input)
	}
}

func BenchmarkUnescape(b *testing.B) {
	input := "~1api~1v1~1users~1~0username"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Unescape(input)
	}
}

func BenchmarkBuild(b *testing.B) {
	keys := []any{"server", "databases", 0, "connection", "host"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Build(keys...)
	}
}

func BenchmarkParse(b *testing.B) {
	pointer := "/server/databases/0/connection/host"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Parse(pointer)
	}
}

func TestGetSetDelete_MapHelpers(t *testing.T) {
	t.Run("GetPath invalid pointer", func(t *testing.T) {
		if _, ok := GetPath(map[string]any{}, "no/leading/slash"); ok {
			t.Fatal("GetPath() ok=true, want false")
		}
	})

	t.Run("GetByKeys supports slices", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a", "b"},
		}
		got, ok := GetByKeys(data, []string{"items", "1"})
		if !ok || got != "b" {
			t.Fatalf("GetByKeys() = (%v, %v), want (%v, true)", got, ok, "b")
		}
	})

	t.Run("GetPath empty pointer returns whole document", func(t *testing.T) {
		data := map[string]any{"a": 1}
		got, ok := GetPath(data, "")
		if !ok || !reflect.DeepEqual(got, data) {
			t.Fatalf("GetPath(\"\") = (%v, %v), want (%v, true)", got, ok, data)
		}
	})

	t.Run("GetByKeys invalid slice index", func(t *testing.T) {
		data := map[string]any{"items": []any{"a"}}
		if _, ok := GetByKeys(data, []string{"items", "x"}); ok {
			t.Fatal("GetByKeys(non-numeric) ok=true, want false")
		}
		if _, ok := GetByKeys(data, []string{"items", "-1"}); ok {
			t.Fatal("GetByKeys(negative) ok=true, want false")
		}
		if _, ok := GetByKeys(data, []string{"items", "9"}); ok {
			t.Fatal("GetByKeys(out of range) ok=true, want false")
		}
	})

	t.Run("GetByKeys stops on non-container types", func(t *testing.T) {
		data := map[string]any{"a": 1}
		if _, ok := GetByKeys(data, []string{"a", "b"}); ok {
			t.Fatal("GetByKeys() ok=true, want false")
		}
	})

	t.Run("MustGetPath panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("MustGetPath() expected panic, got none")
			}
		}()
		_ = MustGetPath(map[string]any{}, "/missing")
	})

	t.Run("MustGetPath returns value", func(t *testing.T) {
		data := map[string]any{"a": 1}
		if got := MustGetPath(data, "/a"); got != 1 {
			t.Fatalf("MustGetPath(/a) = %v, want 1", got)
		}
	})

	t.Run("GetPathOr default", func(t *testing.T) {
		data := map[string]any{"a": 1}
		if got := GetPathOr(data, "/a", 9); got != 1 {
			t.Fatalf("GetPathOr(/a) = %v, want 1", got)
		}
		if got := GetPathOr(data, "/missing", 9); got != 9 {
			t.Fatalf("GetPathOr(/missing) = %v, want 9", got)
		}
	})

	t.Run("SetPath creates maps and overwrites non-map intermediates", func(t *testing.T) {
		data := map[string]any{
			"a": 123, // non-map should be overwritten when setting /a/b
		}
		res := SetPath(data, "/a/b", "x")
		if !res.Success || !res.Created || res.Replaced {
			t.Fatalf("SetPath() result = %+v", res)
		}
		want := map[string]any{"a": map[string]any{"b": "x"}}
		if !reflect.DeepEqual(data, want) {
			t.Fatalf("data = %#v, want %#v", data, want)
		}

		res = SetPath(data, "/a/b", "y")
		if !res.Success || res.Created || !res.Replaced {
			t.Fatalf("SetPath() replace result = %+v", res)
		}
	})

	t.Run("SetByKeys empty keys", func(t *testing.T) {
		if res := SetByKeys(map[string]any{}, nil, 1); res.Success {
			t.Fatalf("SetByKeys(nil keys) Success=true, want false")
		}
	})

	t.Run("SetByKeys traverses existing maps", func(t *testing.T) {
		data := map[string]any{"a": map[string]any{}}
		res := SetByKeys(data, []string{"a", "b"}, "x")
		if !res.Success || !res.Created {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
	})

	t.Run("SetByKeys creates intermediate maps when missing", func(t *testing.T) {
		data := map[string]any{}
		res := SetByKeys(data, []string{"a", "b"}, "x")
		if !res.Success || !res.Created {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
	})

	t.Run("SetPath rejects empty and invalid pointers", func(t *testing.T) {
		if res := SetPath(map[string]any{}, "", 1); res.Success {
			t.Fatalf("SetPath(empty) Success=true, want false")
		}
		if res := SetPath(map[string]any{}, "relative/path", 1); res.Success {
			t.Fatalf("SetPath(relative) Success=true, want false")
		}
	})

	t.Run("DeletePath deletes keys", func(t *testing.T) {
		data := map[string]any{"a": map[string]any{"b": 1}}
		if !DeletePath(data, "/a/b") {
			t.Fatal("DeletePath() = false, want true")
		}
		if DeletePath(data, "/a/b") {
			t.Fatal("DeletePath() = true, want false")
		}
		if DeletePath(data, "") {
			t.Fatal("DeletePath(empty) = true, want false")
		}
	})

	t.Run("DeletePath invalid pointer", func(t *testing.T) {
		if DeletePath(map[string]any{}, "relative/path") {
			t.Fatal("DeletePath(relative) = true, want false")
		}
	})

	t.Run("DeleteByKeys fails on missing or non-map intermediates", func(t *testing.T) {
		if DeleteByKeys(map[string]any{}, nil) {
			t.Fatal("DeleteByKeys(nil) = true, want false")
		}
		data := map[string]any{"a": 1}
		if DeleteByKeys(data, []string{"a", "b"}) {
			t.Fatal("DeleteByKeys(non-map intermediate) = true, want false")
		}
		data = map[string]any{"a": map[string]any{}}
		if DeleteByKeys(data, []string{"a", "missing"}) {
			t.Fatal("DeleteByKeys(missing final key) = true, want false")
		}
		data = map[string]any{}
		if DeleteByKeys(data, []string{"missing", "x"}) {
			t.Fatal("DeleteByKeys(missing intermediate) = true, want false")
		}
	})
}

// TestArrayOperations tests array-related operations in Get/Set/Delete helpers.
func TestArrayOperations(t *testing.T) {
	t.Run("SetByKeys appends to array", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a", "b"},
		}
		res := SetByKeys(data, []string{"items", "2"}, "c")
		if !res.Success || !res.Created {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["items"].([]any)
		if len(items) != 3 || items[2] != "c" {
			t.Fatalf("items = %v, want [a b c]", items)
		}
	})

	t.Run("SetByKeys replaces array element", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a", "b", "c"},
		}
		res := SetByKeys(data, []string{"items", "1"}, "x")
		if !res.Success || !res.Replaced {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["items"].([]any)
		if items[1] != "x" {
			t.Fatalf("items[1] = %v, want x", items[1])
		}
	})

	t.Run("SetByKeys expands array on out of range array index", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a"},
		}
		res := SetByKeys(data, []string{"items", "2"}, "x")
		if !res.Success || !res.Created {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["items"].([]any)
		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		if items[1] != nil {
			t.Errorf("items[1] = %v, want nil", items[1])
		}
		if items[2] != "x" {
			t.Errorf("items[2] = %v, want x", items[2])
		}
	})

	t.Run("SetByKeys fails on negative array index", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a"},
		}
		res := SetByKeys(data, []string{"items", "-1"}, "x")
		if res.Success {
			t.Fatalf("SetByKeys() Success=true, want false")
		}
	})

	t.Run("SetByKeys fails on non-numeric array index", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a"},
		}
		res := SetByKeys(data, []string{"items", "abc"}, "x")
		if res.Success {
			t.Fatalf("SetByKeys() Success=true, want false")
		}
	})

	t.Run("SetByKeys creates intermediate array when next key is numeric", func(t *testing.T) {
		data := map[string]any{}
		res := SetByKeys(data, []string{"items", "0"}, "a")
		if !res.Success || !res.Created {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items, ok := data["items"].([]any)
		if !ok || len(items) != 1 || items[0] != "a" {
			t.Fatalf("data = %#v", data)
		}
	})

	t.Run("SetByKeys navigates through array", func(t *testing.T) {
		data := map[string]any{
			"servers": []any{
				map[string]any{"name": "s1"},
				map[string]any{"name": "s2"},
			},
		}
		res := SetByKeys(data, []string{"servers", "1", "port"}, 8080)
		if !res.Success || !res.Created {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		servers := data["servers"].([]any)
		server1 := servers[1].(map[string]any)
		if server1["port"] != 8080 {
			t.Fatalf("server1 = %v", server1)
		}
	})

	t.Run("SetByKeys replaces non-container element in array with map", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"scalar"},
		}
		res := SetByKeys(data, []string{"items", "0", "key"}, "value")
		if !res.Success {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["items"].([]any)
		item0, ok := items[0].(map[string]any)
		if !ok || item0["key"] != "value" {
			t.Fatalf("items[0] = %v", items[0])
		}
	})

	t.Run("SetByKeys replaces non-container element in array with array", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"scalar"},
		}
		res := SetByKeys(data, []string{"items", "0", "0"}, "nested")
		if !res.Success {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["items"].([]any)
		nested, ok := items[0].([]any)
		if !ok || len(nested) != 1 || nested[0] != "nested" {
			t.Fatalf("items[0] = %v", items[0])
		}
	})

	t.Run("SetByKeys fails on invalid array index during navigation", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a"},
		}
		res := SetByKeys(data, []string{"items", "abc", "key"}, "value")
		if res.Success {
			t.Fatalf("SetByKeys() Success=true, want false")
		}
	})

	t.Run("SetByKeys expands array on out of range array index during navigation", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a"},
		}
		// items[2] (index 1 is skipped) should become a map with "key"="value"
		res := SetByKeys(data, []string{"items", "2", "key"}, "value")
		if !res.Success {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["items"].([]any)
		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		if items[1] != nil {
			t.Errorf("items[1] = %v, want nil", items[1])
		}
		item2, ok := items[2].(map[string]any)
		if !ok || item2["key"] != "value" {
			t.Errorf("items[2] = %v, want map with key=value", items[2])
		}
	})

	t.Run("SetByKeys navigates nested array", func(t *testing.T) {
		data := map[string]any{
			"matrix": []any{
				[]any{"a", "b"},
				[]any{"c", "d"},
			},
		}
		res := SetByKeys(data, []string{"matrix", "1", "0"}, "x")
		if !res.Success || !res.Replaced {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		matrix := data["matrix"].([]any)
		row1 := matrix[1].([]any)
		if row1[0] != "x" {
			t.Fatalf("matrix[1][0] = %v", row1[0])
		}
	})

	t.Run("SetByKeys parent is array with final array index", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a", "b"},
		}
		res := SetPath(data, "/items/1", "x")
		if !res.Success || !res.Replaced {
			t.Fatalf("SetPath() result = %+v", res)
		}
		items := data["items"].([]any)
		if items[1] != "x" {
			t.Fatalf("items[1] = %v", items[1])
		}
	})

	t.Run("SetByKeys appends when parent is array", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a"},
		}
		res := SetPath(data, "/items/1", "b")
		if !res.Success || !res.Created {
			t.Fatalf("SetPath() result = %+v", res)
		}
		items := data["items"].([]any)
		if len(items) != 2 || items[1] != "b" {
			t.Fatalf("items = %v", items)
		}
	})

	t.Run("DeleteByKeys deletes from array", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a", "b", "c"},
		}
		if !DeleteByKeys(data, []string{"items", "1"}) {
			t.Fatal("DeleteByKeys() = false, want true")
		}
		items := data["items"].([]any)
		if len(items) != 2 || items[0] != "a" || items[1] != "c" {
			t.Fatalf("items = %v, want [a c]", items)
		}
	})

	t.Run("DeleteByKeys fails on invalid array index", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a"},
		}
		if DeleteByKeys(data, []string{"items", "abc"}) {
			t.Fatal("DeleteByKeys(non-numeric) = true, want false")
		}
		if DeleteByKeys(data, []string{"items", "-1"}) {
			t.Fatal("DeleteByKeys(negative) = true, want false")
		}
		if DeleteByKeys(data, []string{"items", "5"}) {
			t.Fatal("DeleteByKeys(out of range) = true, want false")
		}
	})

	t.Run("DeleteByKeys navigates through array", func(t *testing.T) {
		data := map[string]any{
			"servers": []any{
				map[string]any{"name": "s1", "port": 80},
			},
		}
		if !DeleteByKeys(data, []string{"servers", "0", "port"}) {
			t.Fatal("DeleteByKeys() = false, want true")
		}
		servers := data["servers"].([]any)
		server0 := servers[0].(map[string]any)
		if _, exists := server0["port"]; exists {
			t.Fatalf("server0 still has port: %v", server0)
		}
	})

	t.Run("DeleteByKeys fails when intermediate array element is not container", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"scalar"},
		}
		if DeleteByKeys(data, []string{"items", "0", "key"}) {
			t.Fatal("DeleteByKeys() = true, want false")
		}
	})

	t.Run("DeleteByKeys navigates nested arrays", func(t *testing.T) {
		data := map[string]any{
			"matrix": []any{
				[]any{"a", "b"},
			},
		}
		if !DeleteByKeys(data, []string{"matrix", "0", "0"}) {
			t.Fatal("DeleteByKeys() = false, want true")
		}
		matrix := data["matrix"].([]any)
		row0 := matrix[0].([]any)
		if len(row0) != 1 || row0[0] != "b" {
			t.Fatalf("matrix[0] = %v, want [b]", row0)
		}
	})

	t.Run("DeleteByKeys fails on invalid array index during navigation", func(t *testing.T) {
		data := map[string]any{
			"items": []any{map[string]any{"k": "v"}},
		}
		if DeleteByKeys(data, []string{"items", "abc", "k"}) {
			t.Fatal("DeleteByKeys(non-numeric) = true, want false")
		}
		if DeleteByKeys(data, []string{"items", "-1", "k"}) {
			t.Fatal("DeleteByKeys(negative) = true, want false")
		}
		if DeleteByKeys(data, []string{"items", "5", "k"}) {
			t.Fatal("DeleteByKeys(out of range) = true, want false")
		}
	})

	t.Run("GetByKeys navigates nested arrays", func(t *testing.T) {
		data := map[string]any{
			"matrix": []any{
				[]any{"a", "b"},
				[]any{"c", "d"},
			},
		}
		val, ok := GetByKeys(data, []string{"matrix", "1", "0"})
		if !ok || val != "c" {
			t.Fatalf("GetByKeys() = (%v, %v), want (c, true)", val, ok)
		}
	})
}

// TestUpdateParentArray tests the updateParentArray helper function indirectly.
func TestUpdateParentArray(t *testing.T) {
	t.Run("append to deeply nested array", func(t *testing.T) {
		data := map[string]any{
			"level1": map[string]any{
				"level2": []any{"a"},
			},
		}
		res := SetPath(data, "/level1/level2/1", "b")
		if !res.Success || !res.Created {
			t.Fatalf("SetPath() result = %+v", res)
		}
		arr := data["level1"].(map[string]any)["level2"].([]any)
		if len(arr) != 2 || arr[1] != "b" {
			t.Fatalf("arr = %v", arr)
		}
	})

	t.Run("append to array inside array", func(t *testing.T) {
		data := map[string]any{
			"matrix": []any{
				[]any{"a"},
			},
		}
		res := SetPath(data, "/matrix/0/1", "b")
		if !res.Success || !res.Created {
			t.Fatalf("SetPath() result = %+v", res)
		}
		matrix := data["matrix"].([]any)
		row0 := matrix[0].([]any)
		if len(row0) != 2 || row0[1] != "b" {
			t.Fatalf("matrix[0] = %v", row0)
		}
	})
}

// TestUpdateParentArrayForDelete tests the updateParentArrayForDelete helper indirectly.
func TestUpdateParentArrayForDelete(t *testing.T) {
	t.Run("delete from deeply nested array", func(t *testing.T) {
		data := map[string]any{
			"level1": map[string]any{
				"level2": []any{"a", "b"},
			},
		}
		if !DeletePath(data, "/level1/level2/0") {
			t.Fatal("DeletePath() = false, want true")
		}
		arr := data["level1"].(map[string]any)["level2"].([]any)
		if len(arr) != 1 || arr[0] != "b" {
			t.Fatalf("arr = %v", arr)
		}
	})

	t.Run("delete from array inside array", func(t *testing.T) {
		data := map[string]any{
			"matrix": []any{
				[]any{"a", "b", "c"},
			},
		}
		if !DeletePath(data, "/matrix/0/1") {
			t.Fatal("DeletePath() = false, want true")
		}
		matrix := data["matrix"].([]any)
		row0 := matrix[0].([]any)
		if len(row0) != 2 || row0[0] != "a" || row0[1] != "c" {
			t.Fatalf("matrix[0] = %v", row0)
		}
	})
}

// TestSetPathSingleKey tests single key operations that use direct map access.
func TestSetPathSingleKey(t *testing.T) {
	t.Run("single key creates new entry", func(t *testing.T) {
		data := map[string]any{}
		res := SetPath(data, "/key", "value")
		if !res.Success || !res.Created || res.Replaced {
			t.Fatalf("SetPath() result = %+v", res)
		}
		if data["key"] != "value" {
			t.Fatalf("data = %v", data)
		}
	})

	t.Run("single key replaces existing", func(t *testing.T) {
		data := map[string]any{"key": "old"}
		res := SetPath(data, "/key", "new")
		if !res.Success || res.Created || !res.Replaced {
			t.Fatalf("SetPath() result = %+v", res)
		}
		if data["key"] != "new" {
			t.Fatalf("data = %v", data)
		}
	})

	t.Run("DeleteByKeys single key", func(t *testing.T) {
		data := map[string]any{"key": "value"}
		if !DeleteByKeys(data, []string{"key"}) {
			t.Fatal("DeleteByKeys() = false, want true")
		}
		if _, exists := data["key"]; exists {
			t.Fatalf("key still exists in data")
		}
	})

	t.Run("DeleteByKeys single key not found", func(t *testing.T) {
		data := map[string]any{}
		if DeleteByKeys(data, []string{"missing"}) {
			t.Fatal("DeleteByKeys() = true, want false")
		}
	})
}

// TestEdgeCases covers remaining edge cases for full coverage.
func TestEdgeCases(t *testing.T) {
	t.Run("SetByKeys with parent key not existing replaces correctly", func(t *testing.T) {
		// Tests the parentKey tracking in SetByKeys when parent exists
		data := map[string]any{
			"a": map[string]any{
				"b": "old",
			},
		}
		res := SetByKeys(data, []string{"a", "b"}, "new")
		if !res.Success || res.Created || !res.Replaced {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
	})

	t.Run("SetByKeys with existing array and numeric final key uses array branch", func(t *testing.T) {
		// Tests the array handling path in SetByKeys where parent is map but has array value
		data := map[string]any{
			"parent": map[string]any{
				"items": []any{"a", "b"},
			},
		}
		res := SetByKeys(data, []string{"parent", "items", "0"}, "x")
		if !res.Success || !res.Replaced {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["parent"].(map[string]any)["items"].([]any)
		if items[0] != "x" {
			t.Fatalf("items[0] = %v", items[0])
		}
	})

	t.Run("SetByKeys with existing array append via nested path", func(t *testing.T) {
		data := map[string]any{
			"parent": map[string]any{
				"items": []any{"a"},
			},
		}
		res := SetByKeys(data, []string{"parent", "items", "1"}, "b")
		if !res.Success || !res.Created {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["parent"].(map[string]any)["items"].([]any)
		if len(items) != 2 || items[1] != "b" {
			t.Fatalf("items = %v", items)
		}
	})

	t.Run("SetByKeys expands array via nested path", func(t *testing.T) {
		data := map[string]any{
			"parent": map[string]any{
				"items": []any{"a"},
			},
		}
		res := SetByKeys(data, []string{"parent", "items", "2"}, "x")
		if !res.Success {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["parent"].(map[string]any)["items"].([]any)
		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		if items[2] != "x" {
			t.Errorf("items[2] = %v, want x", items[2])
		}
	})

	t.Run("navigateToParent with single key", func(t *testing.T) {
		data := map[string]any{}
		res := SetByKeys(data, []string{"key"}, "value")
		if !res.Success {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
	})

	t.Run("updateParentArray with keys length 0", func(t *testing.T) {
		// The updateParentArray is called when appending to array
		// Test the case where the array is at root level
		data := map[string]any{
			"items": []any{"a"},
		}
		res := SetPath(data, "/items/1", "b")
		if !res.Success {
			t.Fatalf("SetPath() result = %+v", res)
		}
	})

	t.Run("updateParentArray traverses nested map then array", func(t *testing.T) {
		data := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"items": []any{"a"},
				},
			},
		}
		res := SetPath(data, "/level1/level2/items/1", "b")
		if !res.Success {
			t.Fatalf("SetPath() result = %+v", res)
		}
		items := data["level1"].(map[string]any)["level2"].(map[string]any)["items"].([]any)
		if len(items) != 2 {
			t.Fatalf("items = %v", items)
		}
	})

	t.Run("updateParentArrayForDelete with zero keys", func(t *testing.T) {
		// This path happens when deleting from top-level array
		data := map[string]any{
			"items": []any{"a", "b"},
		}
		if !DeletePath(data, "/items/0") {
			t.Fatal("DeletePath() = false, want true")
		}
		items := data["items"].([]any)
		if len(items) != 1 || items[0] != "b" {
			t.Fatalf("items = %v", items)
		}
	})

	t.Run("updateParentArrayForDelete traverses array then updates", func(t *testing.T) {
		data := map[string]any{
			"outer": []any{
				map[string]any{
					"inner": []any{"a", "b"},
				},
			},
		}
		if !DeletePath(data, "/outer/0/inner/0") {
			t.Fatal("DeletePath() = false, want true")
		}
		outer := data["outer"].([]any)
		inner := outer[0].(map[string]any)["inner"].([]any)
		if len(inner) != 1 || inner[0] != "b" {
			t.Fatalf("inner = %v", inner)
		}
	})

	t.Run("SetByKeys negative index on nested array path", func(t *testing.T) {
		data := map[string]any{
			"items": []any{"a"},
		}
		// Negative index during navigation should fail
		res := SetByKeys(data, []string{"items", "-1"}, "x")
		if res.Success {
			t.Fatalf("SetByKeys() Success=true, want false")
		}
	})

	t.Run("SetByKeys with existing non-array value and numeric key", func(t *testing.T) {
		// When final key looks like array index but parent value is not array
		data := map[string]any{
			"parent": map[string]any{
				"value": "not an array",
			},
		}
		res := SetByKeys(data, []string{"parent", "value", "0"}, "x")
		// This should create intermediate map for "value" and array for numeric key
		if !res.Success {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
	})

	t.Run("DeleteByKeys default switch case", func(t *testing.T) {
		// The default case in DeleteByKeys when parent is neither map nor array
		// This is difficult to hit directly since navigateToParentForDelete would fail first
		// But let's verify the code path with nested structure
		data := map[string]any{
			"a": map[string]any{
				"b": "value",
			},
		}
		// Try to delete from a path where intermediate is scalar - should return false
		if DeleteByKeys(data, []string{"a", "b", "c"}) {
			t.Fatal("DeleteByKeys() = true, want false")
		}
	})

	t.Run("SetByKeys isArrayParent branch with non-numeric final key", func(t *testing.T) {
		// When parent is array but final key is not a valid index
		data := map[string]any{
			"items": []any{
				map[string]any{},
			},
		}
		// Navigate to array element then try to set with non-numeric key
		res := SetByKeys(data, []string{"items", "0", "key"}, "value")
		if !res.Success {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
	})

	t.Run("SetByKeys isArrayParent with negative final index", func(t *testing.T) {
		// When parent is array and final key is negative index
		data := map[string]any{
			"outer": []any{
				[]any{"a", "b"},
			},
		}
		res := SetByKeys(data, []string{"outer", "0", "-1"}, "x")
		if res.Success {
			t.Fatalf("SetByKeys() Success=true, want false")
		}
	})

	t.Run("SetByKeys isArrayParent with out of range final index expands array", func(t *testing.T) {
		// When parent is array and final key is out of range
		data := map[string]any{
			"outer": []any{
				[]any{"a"},
			},
		}
		res := SetByKeys(data, []string{"outer", "0", "2"}, "x")
		if !res.Success {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		outer := data["outer"].([]any)
		inner := outer[0].([]any)
		if len(inner) != 3 {
			t.Fatalf("len(inner) = %d, want 3", len(inner))
		}
		if inner[2] != "x" {
			t.Errorf("inner[2] = %v, want x", inner[2])
		}
	})

	t.Run("SetByKeys isArrayParent append triggers updateParentArray error", func(t *testing.T) {
		// When appending to nested array
		data := map[string]any{
			"outer": []any{
				[]any{"a"},
			},
		}
		res := SetByKeys(data, []string{"outer", "0", "1"}, "b")
		if !res.Success || !res.Created {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		outer := data["outer"].([]any)
		inner := outer[0].([]any)
		if len(inner) != 2 || inner[1] != "b" {
			t.Fatalf("inner = %v", inner)
		}
	})

	t.Run("updateParentArray traverses array element", func(t *testing.T) {
		// Test updateParentArray when path contains array traversal
		data := map[string]any{
			"level1": []any{
				map[string]any{
					"items": []any{"a"},
				},
			},
		}
		res := SetPath(data, "/level1/0/items/1", "b")
		if !res.Success {
			t.Fatalf("SetPath() result = %+v", res)
		}
		level1 := data["level1"].([]any)
		items := level1[0].(map[string]any)["items"].([]any)
		if len(items) != 2 || items[1] != "b" {
			t.Fatalf("items = %v", items)
		}
	})

	t.Run("updateParentArray updates array element directly", func(t *testing.T) {
		// Test when the parent array itself needs updating
		data := map[string]any{
			"matrix": []any{
				[]any{"a"},
			},
		}
		res := SetPath(data, "/matrix/0/1", "b")
		if !res.Success {
			t.Fatalf("SetPath() result = %+v", res)
		}
	})

	t.Run("updateParentArrayForDelete traverses array", func(t *testing.T) {
		// Test when delete path goes through array element
		data := map[string]any{
			"level1": []any{
				map[string]any{
					"level2": []any{
						map[string]any{
							"items": []any{"a", "b"},
						},
					},
				},
			},
		}
		if !DeletePath(data, "/level1/0/level2/0/items/0") {
			t.Fatal("DeletePath() = false, want true")
		}
		level1 := data["level1"].([]any)
		level2 := level1[0].(map[string]any)["level2"].([]any)
		items := level2[0].(map[string]any)["items"].([]any)
		if len(items) != 1 || items[0] != "b" {
			t.Fatalf("items = %v", items)
		}
	})

	t.Run("updateParentArrayForDelete final is array", func(t *testing.T) {
		// Test when the array to delete from is nested in another array
		data := map[string]any{
			"outer": []any{
				[]any{"x", "y"},
			},
		}
		if !DeletePath(data, "/outer/0/0") {
			t.Fatal("DeletePath() = false, want true")
		}
		outer := data["outer"].([]any)
		inner := outer[0].([]any)
		if len(inner) != 1 || inner[0] != "y" {
			t.Fatalf("inner = %v", inner)
		}
	})

	t.Run("SetByKeys non-numeric final key on map parent with numeric-looking value", func(t *testing.T) {
		// Edge case: final key looks numeric but parent value is not array
		data := map[string]any{
			"config": map[string]any{
				"port": 8080,
			},
		}
		res := SetByKeys(data, []string{"config", "0"}, "value")
		if !res.Success {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		if data["config"].(map[string]any)["0"] != "value" {
			t.Fatalf("data = %v", data)
		}
	})

	t.Run("SetByKeys direct array parent update existing", func(t *testing.T) {
		// Trigger isArrayParent=true branch: parent is array, update existing element
		data := map[string]any{
			"items": []any{"a", "b", "c"},
		}
		// keys=["items", "1"] -> navigateToParent returns items array, isArrayParent=true
		res := SetByKeys(data, []string{"items", "1"}, "x")
		if !res.Success || !res.Replaced {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["items"].([]any)
		if items[1] != "x" {
			t.Fatalf("items = %v", items)
		}
	})

	t.Run("SetByKeys direct array parent append", func(t *testing.T) {
		// Trigger isArrayParent=true branch: parent is array, append new element
		data := map[string]any{
			"items": []any{"a"},
		}
		res := SetByKeys(data, []string{"items", "1"}, "b")
		if !res.Success || !res.Created {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["items"].([]any)
		if len(items) != 2 || items[1] != "b" {
			t.Fatalf("items = %v", items)
		}
	})

	t.Run("SetByKeys direct array parent non-numeric key fails", func(t *testing.T) {
		// Trigger isArrayParent=true branch: parent is array, but final key is not numeric
		data := map[string]any{
			"items": []any{"a"},
		}
		res := SetByKeys(data, []string{"items", "abc"}, "x")
		if res.Success {
			t.Fatalf("SetByKeys() Success=true, want false")
		}
	})

	t.Run("SetByKeys direct array parent negative index fails", func(t *testing.T) {
		// Trigger isArrayParent=true branch: parent is array, negative index
		data := map[string]any{
			"items": []any{"a"},
		}
		res := SetByKeys(data, []string{"items", "-1"}, "x")
		if res.Success {
			t.Fatalf("SetByKeys() Success=true, want false")
		}
	})

	t.Run("SetByKeys direct array parent out of range expands array", func(t *testing.T) {
		// Trigger isArrayParent=true branch: parent is array, index > len
		data := map[string]any{
			"items": []any{"a"},
		}
		res := SetByKeys(data, []string{"items", "2"}, "x")
		if !res.Success {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		items := data["items"].([]any)
		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		if items[2] != "x" {
			t.Errorf("items[2] = %v, want x", items[2])
		}
	})

	t.Run("SetByKeys nested array as parent with append", func(t *testing.T) {
		// Trigger isArrayParent=true with updateParentArray needed
		// The parent is a nested array, and we append to it
		data := map[string]any{
			"matrix": []any{
				[]any{"a"},
			},
		}
		// Navigate: matrix -> matrix[0] (which is []any), then set index 1
		// This should trigger the isArrayParent path AND the updateParentArray call
		res := SetByKeys(data, []string{"matrix", "0", "1"}, "b")
		if !res.Success || !res.Created {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
		matrix := data["matrix"].([]any)
		inner := matrix[0].([]any)
		if len(inner) != 2 || inner[1] != "b" {
			t.Fatalf("inner = %v", inner)
		}
	})

	t.Run("navigateToParent default branch", func(t *testing.T) {
		// Try to trigger the default case in navigateToParent switch
		// This happens when current is neither map nor array
		// This is hard to trigger directly since we start with a map
		data := map[string]any{
			"a": "scalar",
		}
		// This will fail because "a" is a scalar, not traversable
		res := SetByKeys(data, []string{"a", "b", "c"}, "x")
		// The navigateToParent should overwrite "a" with a map
		if !res.Success {
			t.Fatalf("SetByKeys() result = %+v", res)
		}
	})
}

func TestJoin(t *testing.T) {
	tests := []struct {
		name string
		base string
		path string
		want string
	}{
		{
			name: "simple join",
			base: "/server",
			path: "port",
			want: "/server/port",
		},
		{
			name: "path with leading slash",
			base: "/server",
			path: "/port",
			want: "/server/port",
		},
		{
			name: "empty base",
			base: "",
			path: "port",
			want: "/port",
		},
		{
			name: "empty path",
			base: "/server",
			path: "",
			want: "/server",
		},
		{
			name: "both empty",
			base: "",
			path: "",
			want: "",
		},
		{
			name: "nested path",
			base: "/a",
			path: "/b/c",
			want: "/a/b/c",
		},
		{
			name: "base without leading slash",
			base: "server",
			path: "port",
			want: "/server/port",
		},
		{
			name: "deep nesting",
			base: "/profiles/default",
			path: "credential/username",
			want: "/profiles/default/credential/username",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Join(tt.base, tt.path)
			if got != tt.want {
				t.Errorf("Join(%q, %q) = %q, want %q", tt.base, tt.path, got, tt.want)
			}
		})
	}
}
