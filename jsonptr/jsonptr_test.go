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
