package jktest

import (
	"context"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source"
)

// TestSource is an in-memory source that supports both Load and Save.
// It is useful for testing Document implementations via LayerTester.
type TestSource struct {
	data []byte
}

// Ensure TestSource implements source.Source.
var _ source.Source = (*TestSource)(nil)

// Type returns the source type identifier.
func (s *TestSource) Type() source.SourceType {
	return "test"
}

// NewTestSource creates a new TestSource with the given initial data.
func NewTestSource(data []byte) *TestSource {
	return &TestSource{data: data}
}

// Load returns the current data.
func (s *TestSource) Load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]byte, len(s.data))
	copy(result, s.data)
	return result, nil
}

// Save applies the update function and stores the result.
func (s *TestSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	newData, err := updateFunc(s.data)
	if err != nil {
		return err
	}
	s.data = newData
	return nil
}

// CanSave returns true.
func (s *TestSource) CanSave() bool {
	return true
}

// DocumentLayerFactory creates a LayerFactory for testing Document implementations
// via the LayerTester. This is useful when you want to test a Document's behavior
// through the full Layer stack.
//
// Example:
//
//	func TestYAMLDocument_ViaLayer(t *testing.T) {
//	    factory := jktest.DocumentLayerFactory(yaml.New())
//	    jktest.NewLayerTester(t, factory).TestAll()
//	}
func DocumentLayerFactory(doc document.Document) LayerFactory {
	return func(data map[string]any) layer.Layer {
		// Marshal test data to bytes using the document
		bytes, err := doc.MarshalTestData(data)
		if err != nil {
			// Return a layer that will fail on Load
			return layer.New("test", NewTestSource(nil), doc)
		}
		return layer.New("test", NewTestSource(bytes), doc)
	}
}

// DocumentLayerTester wraps LayerTester for testing Document implementations.
// It provides a convenient way to test Document implementations through the
// full Layer stack while also running Document-specific tests.
type DocumentLayerTester struct {
	*LayerTester
	t   *testing.T
	doc document.Document
}

// NewDocumentLayerTester creates a tester for Document implementations.
// It internally creates a LayerTester using DocumentLayerFactory.
//
// Example:
//
//	func TestYAMLDocument_Compliance(t *testing.T) {
//	    jktest.NewDocumentLayerTester(t, yaml.New()).TestAll()
//	}
//
//	func TestTOMLDocument_Compliance(t *testing.T) {
//	    jktest.NewDocumentLayerTester(t, toml.New(),
//	        jktest.SkipNullTest("TOML doesn't support null values"),
//	    ).TestAll()
//	}
func NewDocumentLayerTester(t *testing.T, doc document.Document, opts ...LayerTesterOption) *DocumentLayerTester {
	factory := DocumentLayerFactory(doc)
	return &DocumentLayerTester{
		LayerTester: NewLayerTester(t, factory, opts...),
		t:           t,
		doc:         doc,
	}
}

// TestAll runs all LayerTester tests plus Document-specific tests.
func (dt *DocumentLayerTester) TestAll() {
	dt.LayerTester.TestAll()
	dt.t.Run("Document/Format", dt.testFormat)
}

// testFormat verifies Format() returns a non-empty format type.
func (dt *DocumentLayerTester) testFormat(t *testing.T) {
	format := dt.doc.Format()
	require(t, format != "", "Format() returned empty string")
}
