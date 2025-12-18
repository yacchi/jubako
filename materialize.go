package jubako

import (
	"fmt"

	"github.com/yacchi/jubako/container"
	"github.com/yacchi/jubako/jsonptr"
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
func (s *Store[T]) materializeLocked() (T, []subscriber[T], error) {
	// Clear existing origins
	s.origins.clear()

	if len(s.layers) == 0 {
		// No layers - use zero value
		var zero T
		s.resolved.Set(zero)
		subscribers := append([]subscriber[T](nil), s.subscribers...)
		return zero, subscribers, nil
	}

	// Merge all layers into a single map, tracking origins
	// Layers are already sorted by priority (lowest first)
	merged := make(map[string]any)
	for _, entry := range s.layers {
		if entry.data == nil {
			continue // Skip layers that haven't been loaded
		}

		// Walk the map to track origins for all paths
		walkMapForOrigins("", entry.data, entry, s.origins)

		// Deep merge the layer map into merged
		deepMerge(merged, entry.data)
	}

	// Convert merged map to type T
	// 1. Apply path remapping based on pre-built mapping table (from jubako struct tags)
	remapped := applyMappings(merged, s.schema.Table)

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

