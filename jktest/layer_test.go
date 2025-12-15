package jktest

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/format/json"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/layer/mapdata"
	"github.com/yacchi/jubako/source"
)

func TestLayerTester_MapData_Compliance(t *testing.T) {
	factory := func(data map[string]any) layer.Layer {
		return mapdata.New("test", data)
	}
	NewLayerTester(t, factory).TestAll()
}

func TestLayerTester_SkipOptions(t *testing.T) {
	factory := func(data map[string]any) layer.Layer {
		return mapdata.New("test", data)
	}
	_ = NewLayerTester(t, factory,
		SkipNullTest("test: null not supported"),
		SkipArrayTest("test: arrays not supported"),
	)
}

type readOnlyLayer struct {
	data map[string]any
}

func (l *readOnlyLayer) Name() layer.Name { return "test" }
func (l *readOnlyLayer) Load(ctx context.Context) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if l.data == nil {
		return map[string]any{}, nil
	}
	out := make(map[string]any, len(l.data))
	for k, v := range l.data {
		out[k] = v
	}
	return out, nil
}
func (l *readOnlyLayer) Save(context.Context, document.JSONPatchSet) error {
	return errors.New("read-only")
}
func (l *readOnlyLayer) CanSave() bool { return false }

func TestLayerTester_SkipNullAndArrayAndSave(t *testing.T) {
	t.Run("skip null/array", func(t *testing.T) {
		factory := func(data map[string]any) layer.Layer {
			return mapdata.New("test", data)
		}
		NewLayerTester(t, factory,
			SkipNullTest("test: null not supported"),
			SkipArrayTest("test: arrays not supported"),
		).TestAll()
	})

	t.Run("skip save when read-only", func(t *testing.T) {
		factory := func(data map[string]any) layer.Layer {
			return &readOnlyLayer{data: data}
		}
		NewLayerTester(t, factory).TestAll()
	})
}

func TestMapToEnvVars(t *testing.T) {
	env := MapToEnvVars("APP_", "_", map[string]any{
		"server": map[string]any{
			"host": "localhost",
			"port": 8080,
			"null": nil, // should be skipped
		},
	})

	want := []string{
		"APP_SERVER_HOST=localhost",
		"APP_SERVER_PORT=8080",
	}
	sort.Strings(env)
	sort.Strings(want)
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("MapToEnvVars() = %#v, want %#v", env, want)
	}
}

func TestValueHelpers(t *testing.T) {
	t.Run("isUnsupportedError", func(t *testing.T) {
		if !isUnsupportedError(document.Unsupported("x")) {
			t.Fatal("isUnsupportedError() = false, want true")
		}
		if isUnsupportedError(errors.New("nope")) {
			t.Fatal("isUnsupportedError() = true, want false")
		}
	})

	t.Run("valuesEqual numeric conversion", func(t *testing.T) {
		if !valuesEqual(1, float64(1)) {
			t.Fatal("valuesEqual(1, 1.0) = false, want true")
		}
		if valuesEqual(1, float64(2)) {
			t.Fatal("valuesEqual(1, 2.0) = true, want false")
		}
	})

	t.Run("valuesEqual string numeric representation", func(t *testing.T) {
		if !valuesEqual("42", 42) {
			t.Fatal(`valuesEqual("42", 42) = false, want true`)
		}
	})

	t.Run("toFloat64", func(t *testing.T) {
		if n, ok := toFloat64(int64(1)); !ok || n != 1 {
			t.Fatalf("toFloat64(int64) = (%v, %v), want (1, true)", n, ok)
		}
		if n, ok := toFloat64(float64(1.5)); !ok || n != 1.5 {
			t.Fatalf("toFloat64(float64) = (%v, %v), want (1.5, true)", n, ok)
		}
		if _, ok := toFloat64("x"); ok {
			t.Fatal("toFloat64(string) ok=true, want false")
		}
	})

	t.Run("valuesEqual nil handling", func(t *testing.T) {
		if !valuesEqual(nil, nil) {
			t.Fatal("valuesEqual(nil, nil) = false, want true")
		}
		if valuesEqual(nil, 1) {
			t.Fatal("valuesEqual(nil, 1) = true, want false")
		}
	})

	t.Run("valuesEqual reflect deep equal", func(t *testing.T) {
		if !valuesEqual(map[string]any{"a": 1}, map[string]any{"a": 1}) {
			t.Fatal("valuesEqual(map) = false, want true")
		}
	})
}

type errorDoc struct{}

func (d *errorDoc) Get(_ []byte) (map[string]any, error) { return nil, errors.New("get error") }
func (d *errorDoc) Apply(_ []byte, _ document.JSONPatchSet) ([]byte, error) {
	return nil, errors.New("apply error")
}
func (d *errorDoc) Format() document.DocumentFormat { return "error" }
func (d *errorDoc) MarshalTestData(_ map[string]any) ([]byte, error) {
	return nil, errors.New("marshal error")
}

func TestDocumentLayerFactory_ErrorPath(t *testing.T) {
	factory := DocumentLayerFactory(&errorDoc{})
	l := factory(map[string]any{"a": "b"})
	if _, err := l.Load(context.Background()); err == nil {
		t.Fatal("Load() expected error, got nil")
	}
}

func TestTestSource_Save_UpdateFuncError(t *testing.T) {
	src := NewTestSource([]byte("x"))
	err := src.Save(context.Background(), func(_ []byte) ([]byte, error) {
		return nil, errors.New("boom")
	})
	if err == nil {
		t.Fatal("Save() expected error, got nil")
	}
}

func TestDocumentLayerFactory_Success(t *testing.T) {
	factory := DocumentLayerFactory(json.New())
	l := factory(map[string]any{"a": "b"})
	if _, err := l.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := l.Save(context.Background(), document.JSONPatchSet{document.NewAddPatch("/c", 1)}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

type errUpdateSource struct{}

func (s *errUpdateSource) Load(_ context.Context) ([]byte, error) {
	return nil, errors.New("load error")
}
func (s *errUpdateSource) Save(_ context.Context, _ source.UpdateFunc) error {
	return errors.New("save error")
}
func (s *errUpdateSource) CanSave() bool { return true }

func TestDocumentLayerFactory_LayerLoadFailure(t *testing.T) {
	l := layer.New("test", &errUpdateSource{}, json.New())
	if _, err := l.Load(context.Background()); err == nil {
		t.Fatal("Load() expected error, got nil")
	}
}

type fakeTestT struct {
	fatalCalled bool
	errorCalled bool
	skipCalled  bool
}

func (f *fakeTestT) Helper() {}

func (f *fakeTestT) Fatalf(string, ...any) {
	f.fatalCalled = true
	panic("fatal")
}

func (f *fakeTestT) Errorf(string, ...any) {
	f.errorCalled = true
}

func (f *fakeTestT) Skip(args ...any) {
	f.skipCalled = true
	panic(args)
}

func (f *fakeTestT) Skipf(string, ...any) {
	f.skipCalled = true
	panic("skipf")
}

func TestRequireHelpers(t *testing.T) {
	t.Run("require failure calls Fatalf", func(t *testing.T) {
		ft := &fakeTestT{}
		defer func() { _ = recover() }()
		require(ft, false, "boom")
		if !ft.fatalCalled {
			t.Fatal("require() did not call Fatalf")
		}
	})

	t.Run("requireNoError failure calls Fatalf", func(t *testing.T) {
		ft := &fakeTestT{}
		defer func() { _ = recover() }()
		requireNoError(ft, errors.New("x"), "boom")
		if !ft.fatalCalled {
			t.Fatal("requireNoError() did not call Fatalf")
		}
	})

	t.Run("check failure calls Errorf", func(t *testing.T) {
		ft := &fakeTestT{}
		check(ft, false, "boom")
		if !ft.errorCalled {
			t.Fatal("check() did not call Errorf")
		}
	})
}

func TestTestSource_Branches(t *testing.T) {
	src := NewTestSource([]byte("x"))
	if !src.CanSave() {
		t.Fatal("CanSave() = false, want true")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := src.Load(ctx); err == nil {
		t.Fatal("Load(canceled) expected error, got nil")
	}
	if err := src.Save(ctx, func([]byte) ([]byte, error) { return nil, nil }); err == nil {
		t.Fatal("Save(canceled) expected error, got nil")
	}

	if b, err := src.Load(context.Background()); err != nil || string(b) != "x" {
		t.Fatalf("Load() = (%q, %v), want (\"x\", nil)", string(b), err)
	}

	if err := src.Save(context.Background(), func([]byte) ([]byte, error) { return []byte("y"), nil }); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if b, err := src.Load(context.Background()); err != nil || string(b) != "y" {
		t.Fatalf("Load() after Save = (%q, %v), want (\"y\", nil)", string(b), err)
	}
}
