// Package coordinated provides a layer wrapper for coordinators that reconstruct
// one logical layer from application-owned backing stores.
package coordinated

import (
	"context"
	"fmt"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/types"
	"github.com/yacchi/jubako/watcher"
)

// SchemaView is an alias to the shared layer schema view.
type SchemaView = layer.SchemaView

// PathDescriptor is an alias to the shared schema descriptor interface.
type PathDescriptor = layer.PathDescriptor

// StabilizeContext is an alias to the shared layer stabilization context.
type StabilizeContext = layer.StabilizeContext

// SaveContext is an alias to the shared layer save context.
type SaveContext = layer.SaveContext

// StabilizeResult is an alias to the shared layer stabilization result.
type StabilizeResult = layer.StabilizeResult

// LoadContext provides schema access during coordinator load.
type LoadContext interface {
	Schema() SchemaView
}

// Coordinator reconstructs one logical layer from domain-specific collaborators.
type Coordinator interface {
	Load(ctx context.Context, c LoadContext) (map[string]any, error)
	Stabilize(ctx context.Context, c StabilizeContext) (*StabilizeResult, error)
	Save(ctx context.Context, c SaveContext, changes document.JSONPatchSet) error
}

// WatchCoordinator optionally provides custom watch support.
type WatchCoordinator interface {
	Watch(opts ...layer.WatchOption) (layer.LayerWatcher, error)
}

// DetailsCoordinator optionally fills custom layer details.
type DetailsCoordinator interface {
	FillDetails(*types.Details)
}

// TypeCoordinated is the source identifier reported by coordinated layers.
const TypeCoordinated types.SourceType = "coordinated"

// FormatCoordinated is the format identifier reported by coordinated layers.
const FormatCoordinated types.DocumentFormat = "coordinated"

// New returns a Store-aware layer that initializes a coordinated layer with schema access.
func New(name layer.Name, coordinator Coordinator) layer.Layer {
	return layer.StoreAwareLayerFunc(func(provider layer.StoreProvider) layer.Layer {
		return &Layer{
			name:        name,
			coordinator: coordinator,
			schema:      provider.SchemaView(),
		}
	})
}

// Layer is a coordinated layer implementation.
type Layer struct {
	name        layer.Name
	coordinator Coordinator
	schema      SchemaView
}

var _ layer.Layer = (*Layer)(nil)
var _ layer.SnapshotAwareLayer = (*Layer)(nil)
var _ layer.ContextualSaveLayer = (*Layer)(nil)

func (l *Layer) Name() layer.Name {
	return l.name
}

func (l *Layer) Load(ctx context.Context) (map[string]any, error) {
	return l.coordinator.Load(ctx, loadContext{schema: l.schema})
}

func (l *Layer) Save(ctx context.Context, changes document.JSONPatchSet) error {
	return fmt.Errorf("coordinated layer %q requires Store-managed save context", l.name)
}

func (l *Layer) SaveWithContext(ctx context.Context, c layer.SaveContext, changes document.JSONPatchSet) error {
	return l.coordinator.Save(ctx, c, changes)
}

func (l *Layer) Stabilize(ctx context.Context, c layer.StabilizeContext) (*layer.StabilizeResult, error) {
	return l.coordinator.Stabilize(ctx, c)
}

func (l *Layer) CanSave() bool {
	return true
}

func (l *Layer) FillDetails(d *types.Details) {
	d.Source = TypeCoordinated
	d.Format = FormatCoordinated
	d.Watcher = watcher.TypeNoop
	if filler, ok := l.coordinator.(DetailsCoordinator); ok {
		filler.FillDetails(d)
	}
}

func (l *Layer) Watch(opts ...layer.WatchOption) (layer.LayerWatcher, error) {
	if watcherProvider, ok := l.coordinator.(WatchCoordinator); ok {
		return watcherProvider.Watch(opts...)
	}
	return layer.NewNoopLayerWatcher(), nil
}

type loadContext struct {
	schema SchemaView
}

func (c loadContext) Schema() SchemaView {
	return c.schema
}
