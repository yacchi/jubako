package layer

import (
	"context"
	"reflect"

	"github.com/yacchi/jubako/document"
)

// SchemaView exposes a read-only view of the Store schema to layers.
type SchemaView interface {
	// Lookup returns the descriptor for a concrete JSON Pointer path.
	Lookup(path string) (PathDescriptor, bool)

	// Descriptors returns all schema descriptors known to the Store.
	// Descriptor paths may include wildcard segments for maps and slices.
	Descriptors() []PathDescriptor
}

// PathDescriptor provides read-only metadata about a schema path.
type PathDescriptor interface {
	// Path returns the schema path for this descriptor.
	Path() string

	// FieldKey returns the decoded field key for this descriptor.
	FieldKey() string

	// Sensitive reports whether the field is marked sensitive.
	Sensitive() bool

	// Tag returns the raw struct tag value for the given tag key.
	Tag(key string) (string, bool)

	// StructField returns the underlying struct field metadata.
	StructField() reflect.StructField
}

// StabilizeContext provides the provisional logical snapshot to snapshot-aware layers.
type StabilizeContext interface {
	Snapshot() map[string]any
	Schema() SchemaView
}

// StabilizeResult is returned by snapshot-aware layers during stabilization.
type StabilizeResult struct {
	Data            map[string]any
	Dependencies    []string
	Changed         bool
	ProjectionDirty []string
}

// SnapshotAwareLayer is an optional extension for layers that need stabilization passes.
type SnapshotAwareLayer interface {
	Layer
	Stabilize(ctx context.Context, c StabilizeContext) (*StabilizeResult, error)
}

// SaveContext provides logical before/after views for context-aware save implementations.
type SaveContext interface {
	Logical() map[string]any
	LogicalAfter(changes document.JSONPatchSet) (map[string]any, error)
	Schema() SchemaView
}

// ContextualSaveLayer is an optional extension for layers that need save context.
type ContextualSaveLayer interface {
	Layer
	SaveWithContext(ctx context.Context, c SaveContext, changes document.JSONPatchSet) error
}
