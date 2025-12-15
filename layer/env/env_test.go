package env

import (
	"context"
	"strings"
	"testing"

	"github.com/yacchi/jubako/jktest"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
)

func TestNew(t *testing.T) {
	l := New("env", "TEST_")

	if l.Name() != "env" {
		t.Errorf("Name() = %q, want %q", l.Name(), "env")
	}
	if l.Prefix() != "TEST_" {
		t.Errorf("Prefix() = %q, want %q", l.Prefix(), "TEST_")
	}
}

func TestLayer_Load(t *testing.T) {
	ctx := context.Background()

	t.Run("load environment variables with prefix", func(t *testing.T) {
		// Use t.Setenv for automatic cleanup after test
		t.Setenv("TESTAPP_SERVER_HOST", "localhost")
		t.Setenv("TESTAPP_SERVER_PORT", "8080")
		t.Setenv("TESTAPP_DATABASE_URL", "postgres://localhost/db")

		l := New("env", "TESTAPP_")

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if data == nil {
			t.Fatal("Load() returned nil")
		}

		// Check /server/host
		val, ok := jsonptr.GetPath(data, "/server/host")
		if !ok {
			t.Error("Get(/server/host) should succeed")
		}
		if val != "localhost" {
			t.Errorf("Get(/server/host) = %v, want %q", val, "localhost")
		}

		// Check /server/port
		val, ok = jsonptr.GetPath(data, "/server/port")
		if !ok {
			t.Error("Get(/server/port) should succeed")
		}
		if val != "8080" {
			t.Errorf("Get(/server/port) = %v, want %q", val, "8080")
		}

		// Check /database/url
		val, ok = jsonptr.GetPath(data, "/database/url")
		if !ok {
			t.Error("Get(/database/url) should succeed")
		}
		if val != "postgres://localhost/db" {
			t.Errorf("Get(/database/url) = %v, want %q", val, "postgres://localhost/db")
		}
	})

	t.Run("ignores variables without prefix", func(t *testing.T) {
		t.Setenv("TESTAPP2_VALUE", "included")
		t.Setenv("OTHER_VALUE", "excluded")

		l := New("env", "TESTAPP2_")

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Should have TESTAPP2_VALUE
		val, ok := jsonptr.GetPath(data, "/value")
		if !ok {
			t.Error("Get(/value) should succeed for prefixed variable")
		}
		if val != "included" {
			t.Errorf("Get(/value) = %v, want %q", val, "included")
		}

		// Should NOT have OTHER_VALUE (no prefix match)
		_, ok = jsonptr.GetPath(data, "/other/value")
		if ok {
			t.Error("Get(/other/value) should fail for non-prefixed variable")
		}
	})

	t.Run("empty prefix loads all variables", func(t *testing.T) {
		t.Setenv("TESTALL_VALUE", "test")

		l := New("env", "")

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Should have the variable (path depends on the key structure)
		val, ok := jsonptr.GetPath(data, "/testall/value")
		if !ok {
			t.Error("Get(/testall/value) should succeed with empty prefix")
		}
		if val != "test" {
			t.Errorf("Get(/testall/value) = %v, want %q", val, "test")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()

		l := New("env", "TEST_")

		_, err := l.Load(canceledCtx)
		if err == nil {
			t.Error("Load() should return error with canceled context")
		}
	})
}

func TestDefaultTransform(t *testing.T) {
	t.Run("default delimiter _", func(t *testing.T) {
		transform := DefaultTransform("_")
		tests := []struct {
			key  string
			want string
		}{
			{"SERVER_PORT", "/server/port"},
			{"DATABASE_URL", "/database/url"},
			{"VALUE", "/value"},
			{"A_B_C_D", "/a/b/c/d"},
		}

		for _, tt := range tests {
			t.Run(tt.key, func(t *testing.T) {
				got, _ := transform(tt.key, "value")
				if got != tt.want {
					t.Errorf("DefaultTransform(%q) = %q, want %q", tt.key, got, tt.want)
				}
			})
		}
	})

	t.Run("delimiter __", func(t *testing.T) {
		transform := DefaultTransform("__")
		tests := []struct {
			key  string
			want string
		}{
			{"SERVER__PORT", "/server/port"},
			{"MY_APP__LOG_LEVEL", "/my_app/log_level"},
			{"VALUE", "/value"},
		}

		for _, tt := range tests {
			t.Run(tt.key, func(t *testing.T) {
				got, _ := transform(tt.key, "value")
				if got != tt.want {
					t.Errorf("DefaultTransform(%q) = %q, want %q", tt.key, got, tt.want)
				}
			})
		}
	})

	t.Run("preserves value", func(t *testing.T) {
		transform := DefaultTransform("_")
		path, value := transform("KEY", "test-value")
		if path != "/key" {
			t.Errorf("path = %q, want %q", path, "/key")
		}
		if value != "test-value" {
			t.Errorf("value = %v, want %q", value, "test-value")
		}
	})
}

func TestWithEnvironFunc(t *testing.T) {
	ctx := context.Background()

	t.Run("custom environ function", func(t *testing.T) {
		l := New("env", "APP_", WithEnvironFunc(func() []string {
			return []string{
				"INVALID_ENV_WITHOUT_EQUALS",
				"APP_SERVER_HOST=localhost",
				"APP_SERVER_PORT=8080",
				"OTHER_VAR=ignored",
			}
		}))

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Check values from custom environ
		val, ok := jsonptr.GetPath(data, "/server/host")
		if !ok || val != "localhost" {
			t.Errorf("Get(/server/host) = %v, %v, want 'localhost', true", val, ok)
		}

		val, ok = jsonptr.GetPath(data, "/server/port")
		if !ok || val != "8080" {
			t.Errorf("Get(/server/port) = %v, %v, want '8080', true", val, ok)
		}

		// OTHER_VAR should not be included (no prefix match)
		_, ok = jsonptr.GetPath(data, "/other/var")
		if ok {
			t.Error("OTHER_VAR should not be included")
		}
	})

	t.Run("empty environ function", func(t *testing.T) {
		l := New("env", "APP_", WithEnvironFunc(func() []string {
			return []string{}
		}))

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Root should be empty map
		if len(data) != 0 {
			t.Errorf("Root map should be empty, got %v", data)
		}
	})
}

func TestWithTransformFunc(t *testing.T) {
	ctx := context.Background()

	t.Run("skip variables", func(t *testing.T) {
		l := New("env", "APP_",
			WithEnvironFunc(func() []string {
				return []string{
					"APP_PUBLIC_VALUE=public",
					"APP_INTERNAL_SECRET=secret",
					"APP_PUBLIC_NAME=test",
				}
			}),
			WithTransformFunc(func(key, value string) (string, any) {
				// Skip internal variables
				if strings.HasPrefix(key, "INTERNAL") {
					return "", nil
				}
				// Use default transformation for other keys
				return DefaultTransform("_")(key, value)
			}),
		)

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// PUBLIC_VALUE should exist
		val, ok := jsonptr.GetPath(data, "/public/value")
		if !ok || val != "public" {
			t.Errorf("Get(/public/value) = %v, %v, want 'public', true", val, ok)
		}

		// PUBLIC_NAME should exist
		val, ok = jsonptr.GetPath(data, "/public/name")
		if !ok || val != "test" {
			t.Errorf("Get(/public/name) = %v, %v, want 'test', true", val, ok)
		}

		// INTERNAL_SECRET should be skipped
		_, ok = jsonptr.GetPath(data, "/internal/secret")
		if ok {
			t.Error("INTERNAL_SECRET should be skipped by transform func")
		}
	})

	t.Run("custom path mapping", func(t *testing.T) {
		l := New("env", "APP_",
			WithEnvironFunc(func() []string {
				return []string{
					"APP_SERVER__HOST=localhost",
					"APP_SERVER__PORT=8080",
				}
			}),
			WithTransformFunc(func(key, value string) (string, any) {
				// Custom transformation: use __ as delimiter
				key = strings.ToLower(key)
				parts := strings.Split(key, "__")
				return jsonptr.Build(toAnySlice(parts)...), value
			}),
		)

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// SERVER__HOST -> /server/host
		val, ok := jsonptr.GetPath(data, "/server/host")
		if !ok || val != "localhost" {
			t.Errorf("Get(/server/host) = %v, %v, want 'localhost', true", val, ok)
		}
	})

	t.Run("transform value type", func(t *testing.T) {
		l := New("env", "APP_",
			WithEnvironFunc(func() []string {
				return []string{
					"APP_PORT=8080",
					"APP_DEBUG=true",
				}
			}),
			WithTransformFunc(func(key, value string) (string, any) {
				path := "/" + strings.ToLower(key)
				// Convert PORT to int
				if key == "PORT" {
					return path, 8080
				}
				// Convert DEBUG to bool
				if key == "DEBUG" {
					return path, true
				}
				return path, value
			}),
		)

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		val, ok := jsonptr.GetPath(data, "/port")
		if !ok {
			t.Error("Get(/port) should succeed")
		}
		if v, ok := val.(int); !ok || v != 8080 {
			t.Errorf("Get(/port) = %v (%T), want int 8080", val, val)
		}

		val, ok = jsonptr.GetPath(data, "/debug")
		if !ok {
			t.Error("Get(/debug) should succeed")
		}
		if v, ok := val.(bool); !ok || v != true {
			t.Errorf("Get(/debug) = %v (%T), want bool true", val, val)
		}
	})
}

// toAnySlice converts []string to []any for jsonptr.Build
func toAnySlice(s []string) []any {
	result := make([]any, len(s))
	for i, v := range s {
		result[i] = v
	}
	return result
}

func TestWithDelimiter(t *testing.T) {
	ctx := context.Background()

	t.Run("double underscore delimiter", func(t *testing.T) {
		l := New("env", "APP_",
			WithDelimiter("__"),
			WithEnvironFunc(func() []string {
				return []string{
					"APP_SERVER__HOST=localhost",
					"APP_SERVER__PORT=8080",
					"APP_MY_APP__LOG_LEVEL=debug",
				}
			}),
		)

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// SERVER__HOST -> /server/host
		val, ok := jsonptr.GetPath(data, "/server/host")
		if !ok || val != "localhost" {
			t.Errorf("Get(/server/host) = %v, %v, want 'localhost', true", val, ok)
		}

		// MY_APP__LOG_LEVEL -> /my_app/log_level (underscores preserved)
		val, ok = jsonptr.GetPath(data, "/my_app/log_level")
		if !ok || val != "debug" {
			t.Errorf("Get(/my_app/log_level) = %v, %v, want 'debug', true", val, ok)
		}
	})

	t.Run("dot delimiter", func(t *testing.T) {
		l := New("env", "APP_",
			WithDelimiter("."),
			WithEnvironFunc(func() []string {
				return []string{
					"APP_SERVER.HOST=localhost",
					"APP_SERVER.PORT=8080",
				}
			}),
		)

		data, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		val, ok := jsonptr.GetPath(data, "/server/host")
		if !ok || val != "localhost" {
			t.Errorf("Get(/server/host) = %v, %v, want 'localhost', true", val, ok)
		}
	})
}

func TestDelimiterAccessor(t *testing.T) {
	t.Run("default delimiter", func(t *testing.T) {
		l := New("env", "APP_")
		if l.Delimiter() != "_" {
			t.Errorf("Delimiter() = %q, want '_'", l.Delimiter())
		}
	})

	t.Run("custom delimiter", func(t *testing.T) {
		l := New("env", "APP_", WithDelimiter("__"))
		if l.Delimiter() != "__" {
			t.Errorf("Delimiter() = %q, want '__'", l.Delimiter())
		}
	})
}

func TestLayer_NotWritable(t *testing.T) {
	l := New("env", "TEST_")

	// env.Layer should implement Layer
	_, ok := interface{}(l).(layer.Layer)
	if !ok {
		t.Error("env.Layer should implement Layer")
	}

	// CanSave should return false for env layers
	if l.CanSave() {
		t.Error("env.Layer.CanSave() should return false")
	}
}

func TestLayer_Save(t *testing.T) {
	ctx := context.Background()
	l := New("env", "TEST_")

	// Save should return error for env layers
	err := l.Save(ctx, nil)
	if err == nil {
		t.Error("env.Layer.Save() should return error")
	}
}

// TestLayer_Compliance runs the standard jktest compliance tests.
// Env layers don't support null values or arrays, so those tests are skipped.
func TestLayer_Compliance(t *testing.T) {
	factory := func(data map[string]any) layer.Layer {
		// Use "__" as delimiter to preserve underscores in key names
		envVars := jktest.MapToEnvVars("TEST_", "__", data)
		return New("test", "TEST_", WithDelimiter("__"), WithEnvironFunc(func() []string {
			return envVars
		}))
	}
	jktest.NewLayerTester(t, factory,
		jktest.SkipNullTest("environment variables cannot represent null values"),
		jktest.SkipArrayTest("environment variables cannot represent arrays"),
	).TestAll()
}
