// Package jsonptr provides utilities for working with JSON Pointer (RFC 6901).
//
// JSON Pointer defines a string syntax for identifying a specific value
// within a JSON document. It is used in Jubako for path-based access to
// configuration values.
//
// Reference: https://tools.ietf.org/html/rfc6901
package jsonptr

import (
	"fmt"
	"strconv"
	"strings"
)

// Escape escapes special characters in a key for use in JSON Pointer.
// Per RFC 6901:
//   - "~" is encoded as "~0"
//   - "/" is encoded as "~1"
func Escape(key string) string {
	// Order matters: escape ~ first, then /
	key = strings.ReplaceAll(key, "~", "~0")
	key = strings.ReplaceAll(key, "/", "~1")
	return key
}

// Unescape reverses the escaping applied by Escape.
// Per RFC 6901:
//   - "~1" is decoded as "/"
//   - "~0" is decoded as "~"
func Unescape(key string) string {
	// Order matters: unescape / first, then ~
	key = strings.ReplaceAll(key, "~1", "/")
	key = strings.ReplaceAll(key, "~0", "~")
	return key
}

// Build constructs a JSON Pointer from a sequence of keys.
// Keys can be strings or integers (for array indices).
//
// Examples:
//
//	Build("server", "port")                    -> "/server/port"
//	Build("servers", 0, "name")                -> "/servers/0/name"
//	Build("feature.flags", "enable/disable")   -> "/feature.flags/enable~1disable"
//	Build("paths", "/api/users")               -> "/paths/~1api~1users"
func Build(keys ...any) string {
	if len(keys) == 0 {
		return ""
	}

	var parts []string
	for _, key := range keys {
		var keyStr string
		switch v := key.(type) {
		case string:
			keyStr = v
		case int:
			keyStr = strconv.Itoa(v)
		case int64:
			keyStr = strconv.FormatInt(v, 10)
		case uint:
			keyStr = strconv.FormatUint(uint64(v), 10)
		case uint64:
			keyStr = strconv.FormatUint(v, 10)
		default:
			keyStr = fmt.Sprint(v)
		}
		parts = append(parts, Escape(keyStr))
	}

	return "/" + strings.Join(parts, "/")
}

// Parse splits a JSON Pointer into its component keys.
// Returns an error if the pointer is invalid (does not start with "/").
//
// Examples:
//
//	Parse("/server/port")              -> ["server", "port"], nil
//	Parse("/servers/0/name")           -> ["servers", "0", "name"], nil
//	Parse("/feature.flags/enable~1disable") -> ["feature.flags", "enable/disable"], nil
//	Parse("")                          -> [], nil (empty pointer refers to whole document)
//	Parse("server/port")               -> nil, error (invalid: must start with "/")
func Parse(pointer string) ([]string, error) {
	// Empty string is valid and refers to the whole document
	if pointer == "" {
		return []string{}, nil
	}

	// JSON Pointer must start with "/"
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("invalid JSON Pointer: must start with '/' or be empty")
	}

	// Remove leading "/" and split
	pointer = pointer[1:]
	if pointer == "" {
		// Pointer was just "/" - refers to empty key
		return []string{""}, nil
	}

	parts := strings.Split(pointer, "/")
	keys := make([]string, len(parts))
	for i, part := range parts {
		keys[i] = Unescape(part)
	}

	return keys, nil
}

// Join combines two JSON Pointer paths into one.
// The second path can be either relative (without leading "/") or absolute (with leading "/").
// In both cases, it is appended to the first path.
//
// Examples:
//
//	Join("/server", "port")       -> "/server/port"
//	Join("/server", "/port")      -> "/server/port"
//	Join("", "port")              -> "/port"
//	Join("/server", "")           -> "/server"
//	Join("/a", "/b/c")            -> "/a/b/c"
func Join(base, path string) string {
	// Handle empty cases
	if path == "" {
		if base == "" {
			return ""
		}
		return base
	}

	// Normalize path: remove leading "/" if present
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}

	// Handle empty base
	if base == "" {
		return "/" + path
	}

	// Ensure base starts with "/"
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}

	return base + "/" + path
}
