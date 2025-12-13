package env

import (
	"context"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jktest"
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
	if l.Document() != nil {
		t.Error("Document() should be nil before Load()")
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

		doc, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if doc == nil {
			t.Fatal("Load() returned nil document")
		}

		// Check /server/host
		val, ok := doc.Get("/server/host")
		if !ok {
			t.Error("Get(/server/host) should succeed")
		}
		if val != "localhost" {
			t.Errorf("Get(/server/host) = %v, want %q", val, "localhost")
		}

		// Check /server/port
		val, ok = doc.Get("/server/port")
		if !ok {
			t.Error("Get(/server/port) should succeed")
		}
		if val != "8080" {
			t.Errorf("Get(/server/port) = %v, want %q", val, "8080")
		}

		// Check /database/url
		val, ok = doc.Get("/database/url")
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

		doc, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Should have TESTAPP2_VALUE
		val, ok := doc.Get("/value")
		if !ok {
			t.Error("Get(/value) should succeed for prefixed variable")
		}
		if val != "included" {
			t.Errorf("Get(/value) = %v, want %q", val, "included")
		}

		// Should NOT have OTHER_VALUE (no prefix match)
		_, ok = doc.Get("/other/value")
		if ok {
			t.Error("Get(/other/value) should fail for non-prefixed variable")
		}
	})

	t.Run("empty prefix loads all variables", func(t *testing.T) {
		t.Setenv("TESTALL_VALUE", "test")

		l := New("env", "")

		doc, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Should have the variable (path depends on the key structure)
		val, ok := doc.Get("/testall/value")
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

func TestLayer_GetAndFormat(t *testing.T) {
	ctx := context.Background()

	t.Setenv("TESTDOC_VALUE", "original")

	l := New("env", "TESTDOC_")

	doc, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	t.Run("Get root", func(t *testing.T) {
		val, ok := doc.Get("")
		if !ok {
			t.Error("Get(\"\") should succeed")
		}
		if val == nil {
			t.Error("Get(\"\") should return root map")
		}
	})

	t.Run("Format", func(t *testing.T) {
		if doc.Format() != "env" {
			t.Errorf("Format() = %q, want %q", doc.Format(), "env")
		}
	})
}

func TestEnvKeyToPath(t *testing.T) {
	tests := []struct {
		key  string
		want []string
	}{
		{"SERVER_PORT", []string{"server", "port"}},
		{"DATABASE_URL", []string{"database", "url"}},
		{"VALUE", []string{"value"}},
		{"A_B_C_D", []string{"a", "b", "c", "d"}},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := envKeyToPath(tt.key)
			if len(got) != len(tt.want) {
				t.Errorf("envKeyToPath(%q) = %v, want %v", tt.key, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("envKeyToPath(%q)[%d] = %q, want %q", tt.key, i, got[i], tt.want[i])
				}
			}
		})
	}
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
	err := l.Save(ctx)
	if err == nil {
		t.Error("env.Layer.Save() should return error")
	}
}

// TestEnvDocument_Compliance runs Document compliance tests.
// Tests that are not supported by the env format (Set, Delete, arrays, null values)
// are automatically skipped via UnsupportedStructureError.
func TestEnvDocument_Compliance(t *testing.T) {
	parser := jktest.TestParser("env", func() document.Document {
		return &envDocument{data: make(map[string]any)}
	})
	jktest.NewDocumentTester(t, parser).TestAll()
}
