package jubako

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/yacchi/jubako/container"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
)

// materialize merges all layers into the resolved configuration value.
// This method is called after loading or reloading layers.
// IMPORTANT: Caller must hold the write lock (mu.Lock()).
//
// The merging process:
// 1. Sort layers by priority (lowest first)
// 2. Merge each layer's data into a single map, tracking origins
// 3. Unmarshal the merged map into the configuration type T
// 4. Update the resolved Cell with the new value
// 5. Notify all subscribers
func (s *Store[T]) materializeLocked(ctx context.Context) (T, []subscriber[T], error) {
	if len(s.layers) == 0 {
		// No layers - use zero value
		var zero T
		s.resolved.Set(zero)
		subscribers := append([]subscriber[T](nil), s.subscribers...)
		return zero, subscribers, nil
	}

	if err := s.stabilizeLayersLocked(ctx); err != nil {
		var zero T
		return zero, nil, err
	}

	merged := s.mergeLayerDataLocked(func(entry *layerEntry) map[string]any {
		return entry.data
	})

	// Clear existing origins after stabilization settles.
	s.origins.clear()
	for _, entry := range s.layers {
		if entry.data == nil {
			continue
		}
		walkMapForOrigins("", entry.data, entry, s.origins)
	}

	// Convert merged map to type T
	// 1. Apply path remapping based on pre-built mapping table (from jubako struct tags)
	//    Also applies type conversion using the configured ValueConverter
	remapped := applyMappings(merged, s.schema.Table, s.valueConverter)

	// 2. Decode using the configured decoder
	var result T
	if err := s.decoder(remapped, &result); err != nil {
		var zero T
		return zero, nil, fmt.Errorf("failed to decode merged config: %w", err)
	}

	// Update the resolved value. Subscribers are notified by the caller after locks are released.
	s.resolved.Set(result)
	subscribers := append([]subscriber[T](nil), s.subscribers...)
	return result, subscribers, nil
}

const maxStabilizationPasses = 8

func (s *Store[T]) stabilizeLayersLocked(ctx context.Context) error {
	seen := make(map[string]struct{})

	for pass := 0; pass < maxStabilizationPasses; pass++ {
		state, err := s.stabilizationFingerprintLocked()
		if err != nil {
			return err
		}
		if _, ok := seen[state]; ok {
			return fmt.Errorf("stabilization did not converge: detected oscillation")
		}
		seen[state] = struct{}{}

		snapshot := s.mergeLayerDataLocked(func(entry *layerEntry) map[string]any {
			return entry.data
		})
		schemaView := newStoreSchemaView(s.schema)

		changed := false
		for _, entry := range s.layers {
			stabilizer, ok := entry.layer.(layer.SnapshotAwareLayer)
			if !ok {
				entry.dependencies = nil
				entry.projectionDirty = nil
				s.syncLayerDirty(entry)
				continue
			}

			result, err := stabilizer.Stabilize(ctx, storeStabilizeContext{
				snapshot: snapshot,
				schema:   schemaView,
			})
			if err != nil {
				return fmt.Errorf("failed to stabilize layer %q: %w", entry.layer.Name(), err)
			}

			entry.dependencies = nil
			entry.projectionDirty = nil
			if result != nil {
				entry.dependencies = append(entry.dependencies, result.Dependencies...)
				entry.projectionDirty = normalizeProjectionDirty(result.ProjectionDirty)
				switch {
				case result.Data != nil && (result.Changed || !reflect.DeepEqual(entry.data, result.Data)):
					entry.data = container.DeepCopyMap(result.Data)
					changed = true
				case result.Changed:
					changed = true
				}
			}
			s.syncLayerDirty(entry)
		}

		if !changed {
			return nil
		}
	}

	return fmt.Errorf("stabilization did not converge after %d passes", maxStabilizationPasses)
}

func (s *Store[T]) stabilizationFingerprintLocked() (string, error) {
	type layerState struct {
		Name            string         `json:"name"`
		Data            map[string]any `json:"data,omitempty"`
		Dependencies    []string       `json:"dependencies,omitempty"`
		ProjectionDirty []string       `json:"projection_dirty,omitempty"`
	}

	state := make([]layerState, 0, len(s.layers))
	for _, entry := range s.layers {
		state = append(state, layerState{
			Name:            string(entry.layer.Name()),
			Data:            entry.data,
			Dependencies:    entry.dependencies,
			ProjectionDirty: entry.projectionDirty,
		})
	}

	raw, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("marshal stabilization state: %w", err)
	}
	return string(raw), nil
}

type storeStabilizeContext struct {
	snapshot map[string]any
	schema   layer.SchemaView
}

func (c storeStabilizeContext) Snapshot() map[string]any {
	return container.DeepCopyMap(c.snapshot)
}

func (c storeStabilizeContext) Schema() layer.SchemaView {
	return c.schema
}

// mergeValues merges two values and returns the result.
// For maps, keys are merged recursively.
// For other types (including slices), src replaces dst.
// This is used for dynamic container resolution in GetAt.
func mergeValues(dst, src any) any {
	dstMap, dstIsMap := dst.(map[string]any)
	srcMap, srcIsMap := src.(map[string]any)

	if dstIsMap && srcIsMap {
		// Both are maps - merge recursively into a copy
		result := container.DeepCopyValue(dstMap).(map[string]any)
		deepMerge(result, srcMap)
		return result
	}

	// Not both maps - src replaces dst
	return container.DeepCopyValue(src)
}

// deepMerge performs a deep merge of src into dst.
// For maps, keys are merged recursively.
// For other types, src values replace dst values.
// Values are deep copied to avoid modifying the original src data.
func deepMerge(dst, src map[string]any) {
	for key, srcValue := range src {
		dstValue, exists := dst[key]

		if !exists {
			// Key doesn't exist in dst - deep copy it
			dst[key] = container.DeepCopyValue(srcValue)
			continue
		}

		// Both dst and src have this key - need to merge
		dstMap, dstIsMap := dstValue.(map[string]any)
		srcMap, srcIsMap := srcValue.(map[string]any)

		if dstIsMap && srcIsMap {
			// Both are maps - merge recursively
			deepMerge(dstMap, srcMap)
		} else {
			// One or both are not maps - replace with deep copy
			dst[key] = container.DeepCopyValue(srcValue)
		}
	}
}

// walkMapForOrigins recursively walks a map and records origins for all paths.
// It records both leaf values and container paths.
func walkMapForOrigins(prefix string, data map[string]any, entry *layerEntry, o *origins) {
	for key, value := range data {
		path := prefix + "/" + jsonptr.Escape(key)

		switch v := value.(type) {
		case map[string]any:
			o.setContainer(path, entry)
			walkMapForOrigins(path, v, entry, o)
		case []any:
			o.setContainer(path, entry)
			walkSliceForOrigins(path, v, entry, o)
		default:
			o.setLeaf(path, entry)
		}
	}
}

// walkSliceForOrigins recursively walks a slice and records origins for all paths.
func walkSliceForOrigins(prefix string, data []any, entry *layerEntry, o *origins) {
	for i, value := range data {
		path := fmt.Sprintf("%s/%d", prefix, i)

		switch v := value.(type) {
		case map[string]any:
			o.setContainer(path, entry)
			walkMapForOrigins(path, v, entry, o)
		case []any:
			o.setContainer(path, entry)
			walkSliceForOrigins(path, v, entry, o)
		default:
			o.setLeaf(path, entry)
		}
	}
}
