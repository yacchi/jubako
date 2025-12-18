package layer

import (
	"context"
	"reflect"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/types"
)

// StoreProvider provides store-level information to layers during initialization.
// This interface is implemented by Store and passed to LayerFactory functions.
//
// Layers can use StoreProvider to access configuration that is defined at the Store
// level, enabling dependency inversion - layers can request information from Store
// without directly depending on it.
//
// Not all layers need all methods; each layer uses only the methods it requires.
// Unused methods are simply ignored.
type StoreProvider interface {
	// SchemaType returns the reflect.Type of the Store's configuration struct T.
	// Used by layers like env to build schema mappings from struct tags.
	SchemaType() reflect.Type

	// TagDelimiter returns the delimiter used to separate path and directives
	// in jubako struct tags (default: ",").
	TagDelimiter() string

	// FieldTagName returns the struct tag name used for field name resolution
	// (default: "json").
	FieldTagName() string
}

// StoreAwareLayerFunc is a function that creates a Layer with access to StoreProvider.
// It implements Layer interface so it can be passed to Store.Add directly.
// Store.Add detects StoreAwareLayerFunc via StoreAwareLayerInitializer interface and calls
// InitWithStore to create the actual layer.
//
// The Layer methods (Load, Save, etc.) panic if called directly without
// going through Store.Add, ensuring StoreAwareLayerFunc is always properly initialized.
//
// Example:
//
//	store.Add(layer.StoreAwareLayerFunc(func(p layer.StoreProvider) layer.Layer {
//	    schema := buildSchemaFromType(p.SchemaType())
//	    return env.New("env", "APP_", env.WithTransformFunc(schema.CreateTransformFunc()))
//	}))
type StoreAwareLayerFunc func(StoreProvider) Layer

// Ensure StoreAwareLayerFunc implements StoreAwareLayerInitializer interface.
var _ StoreAwareLayerInitializer = (StoreAwareLayerFunc)(nil)

// StoreAwareLayerInitializer is an interface for layers that need store-aware initialization.
// Store.Add checks for this interface and calls InitWithStore before use.
type StoreAwareLayerInitializer interface {
	Layer
	// InitWithStore initializes the layer with StoreProvider and returns the actual layer.
	InitWithStore(StoreProvider) Layer
}

// InitWithStore implements StoreAwareLayerInitializer.
// It calls the factory function with the provided StoreProvider.
func (f StoreAwareLayerFunc) InitWithStore(provider StoreProvider) Layer {
	return f(provider)
}

// Name returns a placeholder name. This should not be called directly.
func (f StoreAwareLayerFunc) Name() Name {
	return "(uninitialized)"
}

// FillDetails panics because StoreAwareLayerFunc should be initialized via Store.Add.
func (f StoreAwareLayerFunc) FillDetails(d *types.Details) {
	panic("StoreAwareLayerFunc: FillDetails called before InitWithStore - use Store.Add to properly initialize")
}

// Load panics because StoreAwareLayerFunc should be initialized via Store.Add.
func (f StoreAwareLayerFunc) Load(ctx context.Context) (map[string]any, error) {
	panic("StoreAwareLayerFunc: Load called before InitWithStore - use Store.Add to properly initialize")
}

// Save panics because StoreAwareLayerFunc should be initialized via Store.Add.
func (f StoreAwareLayerFunc) Save(ctx context.Context, changeset document.JSONPatchSet) error {
	panic("StoreAwareLayerFunc: Save called before InitWithStore - use Store.Add to properly initialize")
}

// CanSave returns false because StoreAwareLayerFunc is not initialized.
func (f StoreAwareLayerFunc) CanSave() bool {
	return false
}

// Watch panics because StoreAwareLayerFunc should be initialized via Store.Add.
func (f StoreAwareLayerFunc) Watch(opts ...WatchOption) (LayerWatcher, error) {
	panic("StoreAwareLayerFunc: Watch called before InitWithStore - use Store.Add to properly initialize")
}
