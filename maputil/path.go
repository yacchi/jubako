// Package maputil provides utilities for working with map[string]any data structures.
package maputil

import (
	"strconv"

	"github.com/yacchi/jubako/jsonptr"
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

	keys, err := jsonptr.Parse(path)
	if err != nil {
		return nil, false
	}

	return getByKeys(data, keys)
}

// getByKeys traverses the data structure using the given keys.
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
		panic("maputil: path not found: " + path)
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

// SetPath sets a value at the given JSON Pointer path in a nested map.
// Creates intermediate maps as needed.
// Returns true if the value was set successfully.
func SetPath(data map[string]any, path string, value any) bool {
	if path == "" {
		return false
	}

	keys, err := jsonptr.Parse(path)
	if err != nil || len(keys) == 0 {
		return false
	}

	// Navigate to parent
	current := data
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]
		next, ok := current[key]
		if !ok {
			// Create intermediate map
			newMap := make(map[string]any)
			current[key] = newMap
			current = newMap
		} else if m, ok := next.(map[string]any); ok {
			current = m
		} else {
			return false
		}
	}

	// Set the final key
	current[keys[len(keys)-1]] = value
	return true
}
