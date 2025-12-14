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
// Creates intermediate maps as needed. If an intermediate node is not a map,
// it will be replaced with a new map.
// This is useful when you already have the keys from Parse().
// Returns SetResult with details about the operation.
func SetByKeys(data map[string]any, keys []string, value any) SetResult {
	if len(keys) == 0 {
		return SetResult{Success: false}
	}

	current := data
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]
		next, ok := current[key]
		if !ok {
			newMap := make(map[string]any)
			current[key] = newMap
			current = newMap
		} else if m, ok := next.(map[string]any); ok {
			current = m
		} else {
			// Overwrite non-map intermediate
			newMap := make(map[string]any)
			current[key] = newMap
			current = newMap
		}
	}

	// Check if key exists before setting
	finalKey := keys[len(keys)-1]
	_, exists := current[finalKey]

	// Set the final key
	current[finalKey] = value

	return SetResult{
		Success:  true,
		Created:  !exists,
		Replaced: exists,
	}
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
// Returns true if the value was deleted, false if the path was not found.
// This is useful when you already have the keys from Parse().
func DeleteByKeys(data map[string]any, keys []string) bool {
	if len(keys) == 0 {
		return false
	}

	current := data
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]
		next, ok := current[key]
		if !ok {
			return false
		}
		m, ok := next.(map[string]any)
		if !ok {
			return false
		}
		current = m
	}

	// Check if key exists and delete
	finalKey := keys[len(keys)-1]
	if _, exists := current[finalKey]; !exists {
		return false
	}

	delete(current, finalKey)
	return true
}
