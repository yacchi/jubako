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
	"github.com/yacchi/jubako/watcher"
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

// Tests for FillDetails branches

// watchableMemSource is a memSource that implements WatchableSource with NoopWatcher.
type watchableMemSource struct {
	memSource
}

func (s *watchableMemSource) Watch() (watcher.WatcherInitializer, error) {
	return watcher.NewNoop(), nil
}

var _ source.WatchableSource = (*watchableMemSource)(nil)

// detailsFillerSource is a memSource that implements types.DetailsFiller.
type detailsFillerSource struct {
	memSource
	path string
}

func (s *detailsFillerSource) FillDetails(d *types.Details) {
	d.Path = s.path
}

var _ types.DetailsFiller = (*detailsFillerSource)(nil)

func TestFileLayer_FillDetails_WatchableSource(t *testing.T) {
	doc := json.New()
	src := &watchableMemSource{
		memSource: memSource{data: []byte("{}")},
	}
	l := New("test", src, doc)

	details := &types.Details{}
	l.FillDetails(details)

	// Source type should be set
	if details.Source != "mem" {
		t.Errorf("Source = %q, want %q", details.Source, "mem")
	}

	// Format should be set from document
	if details.Format != document.FormatJSON {
		t.Errorf("Format = %q, want %q", details.Format, document.FormatJSON)
	}

	// Watcher type should be noop (from WatchableSource)
	if details.Watcher != watcher.TypeNoop {
		t.Errorf("Watcher = %q, want %q", details.Watcher, watcher.TypeNoop)
	}
}

func TestFileLayer_FillDetails_DetailsFiller(t *testing.T) {
	doc := json.New()
	src := &detailsFillerSource{
		memSource: memSource{data: []byte("{}")},
		path:      "/etc/config.json",
	}
	l := New("test", src, doc)

	details := &types.Details{}
	l.FillDetails(details)

	// Path should be set from source's FillDetails
	if details.Path != "/etc/config.json" {
		t.Errorf("Path = %q, want %q", details.Path, "/etc/config.json")
	}
}

func TestFileLayer_FillDetails_NonWatchableSource(t *testing.T) {
	doc := json.New()
	src := &memSource{data: []byte("{}")}
	l := New("test", src, doc)

	details := &types.Details{}
	l.FillDetails(details)

	// Watcher type should be polling (fallback for non-WatchableSource)
	if details.Watcher != watcher.TypePolling {
		t.Errorf("Watcher = %q, want %q", details.Watcher, watcher.TypePolling)
	}
}

// watchableErrorSource returns an error from Watch.
type watchableErrorSource struct {
	memSource
}

func (s *watchableErrorSource) Watch() (watcher.WatcherInitializer, error) {
	return nil, errors.New("watch error")
}

var _ source.WatchableSource = (*watchableErrorSource)(nil)

func TestFileLayer_FillDetails_WatchError(t *testing.T) {
	doc := json.New()
	src := &watchableErrorSource{
		memSource: memSource{data: []byte("{}")},
	}
	l := New("test", src, doc)

	details := &types.Details{}
	l.FillDetails(details)

	// Watcher type should be empty when Watch() returns an error
	// (but Source and Format should still be set)
	if details.Source != "mem" {
		t.Errorf("Source = %q, want %q", details.Source, "mem")
	}
	if details.Format != document.FormatJSON {
		t.Errorf("Format = %q, want %q", details.Format, document.FormatJSON)
	}
}

// watchableInitErrorSource returns an initializer that errors.
type watchableInitErrorSource struct {
	memSource
}

func (s *watchableInitErrorSource) Watch() (watcher.WatcherInitializer, error) {
	return func(params watcher.WatcherInitializerParams) (watcher.Watcher, error) {
		return nil, errors.New("init error")
	}, nil
}

var _ source.WatchableSource = (*watchableInitErrorSource)(nil)

func TestFileLayer_FillDetails_WatchInitError(t *testing.T) {
	doc := json.New()
	src := &watchableInitErrorSource{
		memSource: memSource{data: []byte("{}")},
	}
	l := New("test", src, doc)

	details := &types.Details{}
	l.FillDetails(details)

	// Watcher type should be empty when WatcherInitializer returns an error
	if details.Watcher != "" {
		t.Errorf("Watcher = %q, want empty", details.Watcher)
	}
}
