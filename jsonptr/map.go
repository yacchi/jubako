package jsonptr

import (
	"strconv"
)

// GetPath retrieves a value at the given JSON Pointer path from a nested map.
// Returns the value and true if found, or nil and false if not found.
//
// Example:
//
//	data := map[string]any{
//	    "server": map[string]any{
//	        "host": "localhost",
//	        "port": 8080,
//	    },
//	}
//	value, ok := GetPath(data, "/server/host")  // "localhost", true
//	value, ok := GetPath(data, "/server/missing")  // nil, false
func GetPath(data map[string]any, path string) (any, bool) {
	if path == "" {
		return data, true
	}

	keys, err := Parse(path)
	if err != nil {
		return nil, false
	}

	return GetByKeys(data, keys)
}

// GetByKeys retrieves a value from a nested map using pre-parsed keys.
// This is useful when you already have the keys from Parse() and want to
// avoid re-parsing or when manipulating the keys slice directly.
func GetByKeys(data map[string]any, keys []string) (any, bool) {
	return getByKeys(data, keys)
}

// getByKeys traverses the data structure using the given keys.
// This internal function accepts any type to handle both map and slice traversal.
func getByKeys(data any, keys []string) (any, bool) {
	current := data

	for _, key := range keys {
		switch v := current.(type) {
		case map[string]any:
			val, ok := v[key]
			if !ok {
				return nil, false
			}
			current = val

		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(v) {
				return nil, false
			}
			current = v[index]

		default:
			return nil, false
		}
	}

	return current, true
}

// MustGetPath retrieves a value at the given JSON Pointer path.
// Panics if the path is not found.
func MustGetPath(data map[string]any, path string) any {
	value, ok := GetPath(data, path)
	if !ok {
		panic("jsonptr: path not found: " + path)
	}
	return value
}

// GetPathOr retrieves a value at the given JSON Pointer path.
// Returns the default value if the path is not found.
func GetPathOr(data map[string]any, path string, defaultValue any) any {
	value, ok := GetPath(data, path)
	if !ok {
		return defaultValue
	}
	return value
}

// SetResult contains the result of a SetPath operation.
type SetResult struct {
	// Success indicates whether the value was set successfully.
	Success bool
	// Created indicates this was a new key (add operation).
	// Only meaningful when Success is true.
	Created bool
	// Replaced indicates an existing key was updated (replace operation).
	// Only meaningful when Success is true.
	Replaced bool
}

// SetPath sets a value at the given JSON Pointer path in a nested map.
// Creates intermediate maps as needed. If an intermediate node is not a map,
// it will be replaced with a new map.
// Returns SetResult with details about the operation.
func SetPath(data map[string]any, path string, value any) SetResult {
	if path == "" {
		return SetResult{Success: false}
	}

	keys, err := Parse(path)
	if err != nil || len(keys) == 0 {
		return SetResult{Success: false}
	}

	return SetByKeys(data, keys, value)
}

// SetByKeys sets a value in a nested map using pre-parsed keys.
// Creates intermediate maps as needed. Supports array index operations.
// For arrays, you can:
//   - Update an existing element: index < len(array)
//   - Append a new element: index == len(array)
//
// This is useful when you already have the keys from Parse().
// Returns SetResult with details about the operation.
func SetByKeys(data map[string]any, keys []string, value any) SetResult {
	if len(keys) == 0 {
		return SetResult{Success: false}
	}

	// Handle single key case (direct map access)
	if len(keys) == 1 {
		finalKey := keys[0]
		_, exists := data[finalKey]
		data[finalKey] = value
		return SetResult{
			Success:  true,
			Created:  !exists,
			Replaced: exists,
		}
	}

	// Navigate to the parent of the final key
	parent, parentKey, isArrayParent, ok := navigateToParent(data, keys)
	if !ok {
		return SetResult{Success: false}
	}

	finalKey := keys[len(keys)-1]

	if isArrayParent {
		// Parent is an array
		arr := parent.([]any)
		idx, err := strconv.Atoi(finalKey)
		if err != nil {
			return SetResult{Success: false}
		}
		if idx < 0 {
			return SetResult{Success: false}
		}

		if idx >= len(arr) {
			// Expand array
			newArr := make([]any, idx+1)
			copy(newArr, arr)
			newArr[idx] = value

			// Update the parent container with the new array
			if err := updateParentArray(data, keys[:len(keys)-1], newArr); err != nil {
				return SetResult{Success: false}
			}
			return SetResult{Success: true, Created: true}
		}

		// Update existing element
		arr[idx] = value
		return SetResult{Success: true, Replaced: true}
	}

	// Parent is a map
	m := parent.(map[string]any)
	_, exists := m[parentKey]

	// Check if final key is an array index for a non-existent array
	if idx, err := strconv.Atoi(finalKey); err == nil {
		existing, hasKey := m[parentKey]
		if hasKey {
			if arr, isArr := existing.([]any); isArr {
				if idx < 0 || idx > len(arr) {
					return SetResult{Success: false}
				}
				if idx == len(arr) {
					m[parentKey] = append(arr, value)
					return SetResult{Success: true, Created: true}
				}
				arr[idx] = value
				return SetResult{Success: true, Replaced: true}
			}
		}
	}

	_, finalExists := m[finalKey]
	m[finalKey] = value
	return SetResult{
		Success:  true,
		Created:  !finalExists && !exists,
		Replaced: finalExists || exists,
	}
}

// navigateToParent navigates to the parent container of the final key.
// Returns (parent container, parent key in grandparent, isArray, ok).
func navigateToParent(data map[string]any, keys []string) (any, string, bool, bool) {
	if len(keys) < 2 {
		return data, "", false, true
	}

	var current any = data
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]

		switch c := current.(type) {
		case map[string]any:
			next, ok := c[key]
			if !ok {
				// Need to create intermediate
				// Check if next key is numeric (create array) or not (create map)
				if i+1 < len(keys) {
					if _, err := strconv.Atoi(keys[i+1]); err == nil {
						// Next key is numeric, create array
						newArr := make([]any, 0)
						c[key] = newArr
						current = newArr
						continue
					}
				}
				newMap := make(map[string]any)
				c[key] = newMap
				current = newMap
			} else if m, ok := next.(map[string]any); ok {
				current = m
			} else if arr, ok := next.([]any); ok {
				current = arr
			} else {
				// Overwrite non-container intermediate with map
				newMap := make(map[string]any)
				c[key] = newMap
				current = newMap
			}
		case []any:
			idx, err := strconv.Atoi(key)
			if err != nil || idx < 0 {
				return nil, "", false, false
			}

			if idx >= len(c) {
				// Expand array
				newArr := make([]any, idx+1)
				copy(newArr, c)

				// Update parent with expanded array
				// The path to the current array is keys[:i]
				if err := updateParentArray(data, keys[:i], newArr); err != nil {
					return nil, "", false, false
				}
				c = newArr
				current = newArr
			}

			elem := c[idx]
			if m, ok := elem.(map[string]any); ok {
				current = m
			} else if arr, ok := elem.([]any); ok {
				current = arr
			} else {
				// Need to replace with container
				// Check if next key is numeric
				if i+1 < len(keys) {
					if _, err := strconv.Atoi(keys[i+1]); err == nil {
						newArr := make([]any, 0)
						c[idx] = newArr
						current = newArr
						continue
					}
				}
				newMap := make(map[string]any)
				c[idx] = newMap
				current = newMap
			}
		default:
			return nil, "", false, false
		}
	}

	// Determine if parent is array
	if _, isArr := current.([]any); isArr {
		return current, keys[len(keys)-2], true, true
	}
	return current, keys[len(keys)-2], false, true
}

// updateParentArray updates an array at the given path in the data structure.
func updateParentArray(data map[string]any, keys []string, newArr []any) error {
	if len(keys) == 0 {
		return nil
	}

	if len(keys) == 1 {
		data[keys[0]] = newArr
		return nil
	}

	var current any = data
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]
		switch c := current.(type) {
		case map[string]any:
			current = c[key]
		case []any:
			idx, _ := strconv.Atoi(key)
			current = c[idx]
		}
	}

	lastKey := keys[len(keys)-1]
	switch c := current.(type) {
	case map[string]any:
		c[lastKey] = newArr
	case []any:
		idx, _ := strconv.Atoi(lastKey)
		c[idx] = newArr
	}
	return nil
}

// DeletePath removes a value at the given JSON Pointer path from a nested map.
// Returns true if the value was deleted, false if the path was not found.
//
// Example:
//
//	data := map[string]any{
//	    "server": map[string]any{
//	        "host": "localhost",
//	        "port": 8080,
//	    },
//	}
//	deleted := DeletePath(data, "/server/port")  // true
//	deleted = DeletePath(data, "/server/missing")  // false
func DeletePath(data map[string]any, path string) bool {
	if path == "" {
		return false
	}

	keys, err := Parse(path)
	if err != nil || len(keys) == 0 {
		return false
	}

	return DeleteByKeys(data, keys)
}

// DeleteByKeys removes a value from a nested map using pre-parsed keys.
// Supports array index operations - when deleting from an array,
// the element is removed and subsequent elements shift down.
// Returns true if the value was deleted, false if the path was not found.
// This is useful when you already have the keys from Parse().
func DeleteByKeys(data map[string]any, keys []string) bool {
	if len(keys) == 0 {
		return false
	}

	// Handle single key case
	if len(keys) == 1 {
		finalKey := keys[0]
		if _, exists := data[finalKey]; !exists {
			return false
		}
		delete(data, finalKey)
		return true
	}

	// Navigate to the parent container
	parent, ok := navigateToParentForDelete(data, keys)
	if !ok || parent == nil {
		return false
	}

	finalKey := keys[len(keys)-1]

	switch p := parent.(type) {
	case map[string]any:
		if _, exists := p[finalKey]; !exists {
			return false
		}
		delete(p, finalKey)
		return true
	case []any:
		idx, err := strconv.Atoi(finalKey)
		if err != nil || idx < 0 || idx >= len(p) {
			return false
		}
		// Remove the element from the array
		newArr := append(p[:idx], p[idx+1:]...)
		// Update the parent container with the new array
		updateParentArrayForDelete(data, keys[:len(keys)-1], newArr)
		return true
	}

	return false
}

// navigateToParentForDelete navigates to the parent container.
// Returns (parent container, ok).
func navigateToParentForDelete(data map[string]any, keys []string) (any, bool) {
	if len(keys) < 2 {
		return data, true
	}

	var current any = data
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]

		switch c := current.(type) {
		case map[string]any:
			next, ok := c[key]
			if !ok {
				return nil, false
			}
			if m, ok := next.(map[string]any); ok {
				current = m
			} else if arr, ok := next.([]any); ok {
				current = arr
			} else {
				return nil, false
			}
		case []any:
			idx, err := strconv.Atoi(key)
			if err != nil || idx < 0 || idx >= len(c) {
				return nil, false
			}
			elem := c[idx]
			if m, ok := elem.(map[string]any); ok {
				current = m
			} else if arr, ok := elem.([]any); ok {
				current = arr
			} else {
				return nil, false
			}
		default:
			return nil, false
		}
	}

	return current, true
}

// updateParentArrayForDelete updates an array at the given path after deletion.
func updateParentArrayForDelete(data map[string]any, keys []string, newArr []any) {
	if len(keys) == 0 {
		return
	}

	if len(keys) == 1 {
		data[keys[0]] = newArr
		return
	}

	var current any = data
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]
		switch c := current.(type) {
		case map[string]any:
			current = c[key]
		case []any:
			idx, _ := strconv.Atoi(key)
			current = c[idx]
		}
	}

	lastKey := keys[len(keys)-1]
	switch c := current.(type) {
	case map[string]any:
		c[lastKey] = newArr
	case []any:
		idx, _ := strconv.Atoi(lastKey)
		c[idx] = newArr
	}
}
