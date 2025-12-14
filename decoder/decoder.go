// Package decoder provides map-to-struct decoding functions for jubako.
package decoder

// Func is a function that decodes a map[string]any into a target struct.
// Implementations should handle type conversion as needed.
type Func func(data map[string]any, target any) error
