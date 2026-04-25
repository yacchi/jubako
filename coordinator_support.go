package jubako

import (
	"sort"
	"strings"

	"github.com/yacchi/jubako/container"
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
)

func (s *Store[T]) syncLayerDirty(entry *layerEntry) {
	entry.dirty = !entry.changeset.IsEmpty() || len(entry.projectionDirty) > 0
}

func normalizeProjectionDirty(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func normalizeSavePatches(base document.JSONPatchSet, dirty []string, current map[string]any) document.JSONPatchSet {
	if len(dirty) == 0 {
		return base
	}

	patches := append(document.JSONPatchSet(nil), base...)
	for _, root := range normalizeProjectionDirty(dirty) {
		if patchSetTouchesPath(patches, root) {
			continue
		}
		value, ok := jsonptr.GetPath(current, root)
		if !ok {
			continue
		}
		patches = append(patches, document.NewReplacePatch(root, container.DeepCopyValue(value)))
	}
	return patches
}

func patchSetTouchesPath(patches document.JSONPatchSet, root string) bool {
	for _, patch := range patches {
		if patch.Path == root || strings.HasPrefix(patch.Path, root+"/") {
			return true
		}
	}
	return false
}

func (s *Store[T]) mergeLayerDataLocked(selectData func(*layerEntry) map[string]any) map[string]any {
	merged := make(map[string]any)
	for _, entry := range s.layers {
		data := selectData(entry)
		if data == nil {
			continue
		}
		deepMerge(merged, data)
	}
	return merged
}

type layerSaveContext[T any] struct {
	store  *Store[T]
	target *layerEntry
	schema layer.SchemaView
}

func (s *Store[T]) newLayerSaveContext(entry *layerEntry) layer.SaveContext {
	return layerSaveContext[T]{
		store:  s,
		target: entry,
		schema: newStoreSchemaView(s.schema),
	}
}

func (c layerSaveContext[T]) Logical() map[string]any {
	return c.store.mergeLayerDataLocked(func(entry *layerEntry) map[string]any {
		if entry == c.target {
			return entry.loadedData
		}
		return entry.data
	})
}

func (c layerSaveContext[T]) LogicalAfter(changes document.JSONPatchSet) (map[string]any, error) {
	return c.store.mergeLayerDataLocked(func(entry *layerEntry) map[string]any {
		return entry.data
	}), nil
}

func (c layerSaveContext[T]) Schema() layer.SchemaView {
	return c.schema
}
