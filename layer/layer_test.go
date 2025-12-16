package layer

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/format/json"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/types"
)

type memSource struct {
	data    []byte
	canSave bool
	loadErr error
}

func (s *memSource) Type() source.SourceType { return "mem" }

func (s *memSource) Load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	b := make([]byte, len(s.data))
	copy(b, s.data)
	return b, nil
}

func (s *memSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.canSave {
		return source.ErrSaveNotSupported
	}
	newData, err := updateFunc(s.data)
	if err != nil {
		return err
	}
	s.data = newData
	return nil
}

func (s *memSource) CanSave() bool { return s.canSave }

func TestFileLayer_LoadSaveAndMetadata(t *testing.T) {
	doc := json.New()
	src := &memSource{
		data:    []byte("{\"a\": 1}\n"),
		canSave: true,
	}

	l := New("test", src, doc)

	if l.Name() != "test" {
		t.Fatalf("Name() = %q, want %q", l.Name(), "test")
	}
	if !l.CanSave() {
		t.Fatal("CanSave() = false, want true")
	}
	// Layer interface now includes DetailsFiller, so FillDetails can be called directly
	details := &types.Details{}
	l.FillDetails(details)
	// Check Format is set from document
	if details.Format != document.FormatJSON {
		t.Fatalf("FillDetails() Format = %q, want %q", details.Format, document.FormatJSON)
	}
	// memSource doesn't implement DetailsFiller, so details.Path should be empty
	if details.Path != "" {
		t.Fatalf("FillDetails() Path = %q, want empty", details.Path)
	}
	dp, ok := l.(DocumentProvider)
	if !ok {
		t.Fatal("layer does not implement DocumentProvider")
	}
	if dp.Document() != doc {
		t.Fatal("Document() did not return the original document")
	}

	loaded, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(loaded, map[string]any{"a": float64(1)}) {
		t.Fatalf("Load() = %#v", loaded)
	}

	var patches document.JSONPatchSet
	patches.Add("/b", "x")
	if err := l.Save(context.Background(), patches); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err = l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() after Save error = %v", err)
	}
	if loaded["b"] != "x" {
		t.Fatalf("after Save, b = %#v, want %q", loaded["b"], "x")
	}
}

func TestFileLayer_CanSaveFalse(t *testing.T) {
	doc := json.New()
	src := &memSource{
		data:    []byte("{\"a\": 1}\n"),
		canSave: false,
	}
	l := New("test", src, doc)
	if l.CanSave() {
		t.Fatal("CanSave() = true, want false")
	}
}

type errDoc struct{}

func (d *errDoc) Get(_ []byte) (map[string]any, error) { return nil, errors.New("get error") }
func (d *errDoc) Apply(_ []byte, _ document.JSONPatchSet) ([]byte, error) {
	return nil, errors.New("apply error")
}
func (d *errDoc) Format() document.DocumentFormat { return "error" }
func (d *errDoc) MarshalTestData(_ map[string]any) ([]byte, error) {
	return nil, errors.New("marshal error")
}

func TestFileLayer_Load_ErrorPaths(t *testing.T) {
	t.Run("source load error", func(t *testing.T) {
		doc := json.New()
		src := &memSource{loadErr: errors.New("load error")}
		l := New("test", src, doc)

		if _, err := l.Load(context.Background()); err == nil {
			t.Fatal("Load() expected error, got nil")
		}
	})

	t.Run("document get error", func(t *testing.T) {
		doc := &errDoc{}
		src := &memSource{data: []byte("x"), canSave: true}
		l := New("test", src, doc)

		if _, err := l.Load(context.Background()); err == nil {
			t.Fatal("Load() expected error, got nil")
		}
	})
}
