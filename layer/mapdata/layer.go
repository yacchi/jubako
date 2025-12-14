// Package mapdata provides a map-based configuration layer.
// This layer is useful for testing and programmatic configuration injection
// without requiring any format-specific dependencies (YAML, TOML, etc.).
//
// The layer operates directly on map[string]any data structures, making it
// ideal for unit tests where you want to inject configuration without parsing.
package mapdata

import (
	"context"

	"github.com/yacchi/jubako/container"
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
)

// Layer is a configuration layer backed by a map[string]any.
// It supports read/write operations and is useful for testing and
// programmatic configuration.
type Layer struct {
	name layer.Name
	data map[string]any
}

// Ensure Layer implements layer.Layer interface.
var _ layer.Layer = (*Layer)(nil)

// New creates a new mapdata layer with the given data.
// The data map is deep-copied, so modifications to the original map
// will not affect the layer.
//
// Example:
//
//	data := map[string]any{
//	    "server": map[string]any{
//	        "host": "localhost",
//	        "port": 8080,
//	    },
//	}
//	layer := mapdata.New("test", data)
func New(name layer.Name, data map[string]any) *Layer {
	if data == nil {
		data = make(map[string]any)
	}
	return &Layer{
		name: name,
		data: container.DeepCopyMap(data),
	}
}

// Name returns the layer's name.
func (l *Layer) Name() layer.Name {
	return l.name
}

// Load returns a deep copy of the layer's data.
func (l *Layer) Load(ctx context.Context) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Return a deep copy to prevent external modifications
	return container.DeepCopyMap(l.data), nil
}

// Save applies the changeset to the layer's internal data.
// Since mapdata layers don't have an underlying file format, the changeset
// is applied directly to the in-memory data structure.
func (l *Layer) Save(ctx context.Context, changeset document.JSONPatchSet) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Apply each patch operation to the internal data
	for _, patch := range changeset {
		switch patch.Op {
		case document.PatchOpAdd, document.PatchOpReplace:
			jsonptr.SetPath(l.data, patch.Path, patch.Value)
		case document.PatchOpRemove:
			jsonptr.DeletePath(l.data, patch.Path)
		}
	}
	return nil
}

// CanSave returns true because mapdata layers support in-memory modifications.
// Note: Changes are not persisted to any external storage.
func (l *Layer) CanSave() bool {
	return true
}

// Data returns a deep copy of the current layer data.
func (l *Layer) Data() map[string]any {
	return container.DeepCopyMap(l.data)
}
