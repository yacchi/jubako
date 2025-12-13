// Package mapdoc provides a basic Document implementation backed by map[string]any.
//
// This is intended for implementations that naturally operate on map[string]any
// (e.g., using the standard encoding/json package) and only need basic read/write
// access without format-specific AST handling.
package mapdoc

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
)

// Get returns the value at keys within a nested structure rooted at map[string]any.
// It traverses maps, and also supports indexing into []any when a key is a numeric string.
func Get(root map[string]any, keys []string) (any, bool) {
	current := any(root)
	for _, key := range keys {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[key]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index, err := parseArrayIndex(key)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			rv := reflect.ValueOf(current)
			if !rv.IsValid() {
				return nil, false
			}

			// Support typed maps like map[string]string
			if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
				v := rv.MapIndex(reflect.ValueOf(key))
				if !v.IsValid() {
					return nil, false
				}
				current = v.Interface()
				continue
			}

			// Support typed slices like []string (but treat []byte as scalar)
			if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() != reflect.Uint8 {
				index, err := parseArrayIndex(key)
				if err != nil || index < 0 || index >= rv.Len() {
					return nil, false
				}
				current = rv.Index(index).Interface()
				continue
			}

			return nil, false
		}
	}
	return current, true
}

func parseArrayIndex(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("array index must be non-empty")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("array index must be numeric: %q", s)
		}
	}
	return strconv.Atoi(s)
}

// Set sets value at keys within a nested map[string]any structure.
// It creates intermediate maps as needed and overwrites non-map intermediates.
func Set(root map[string]any, keys []string, value any) {
	if len(keys) == 0 {
		return
	}

	current := root
	for _, key := range keys[:len(keys)-1] {
		existing, ok := current[key]
		if nested, okNested := existing.(map[string]any); ok && okNested {
			current = nested
			continue
		}

		newMap := make(map[string]any)
		current[key] = newMap
		current = newMap
	}

	current[keys[len(keys)-1]] = value
}

// Delete removes the value at keys from a nested map[string]any structure.
// It is idempotent: missing paths do not error.
func Delete(root map[string]any, keys []string) {
	if len(keys) == 0 {
		return
	}

	current := root
	for _, key := range keys[:len(keys)-1] {
		existing, ok := current[key]
		if !ok {
			return
		}
		nested, ok := existing.(map[string]any)
		if !ok {
			return
		}
		current = nested
	}

	delete(current, keys[len(keys)-1])
}

// DeepCopyMap creates a deep copy of a map[string]any (including nested maps and []any).
func DeepCopyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}

	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = deepCopyValue(v)
	}
	return dst
}

func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return DeepCopyMap(val)
	case []any:
		dst := make([]any, len(val))
		for i, elem := range val {
			dst[i] = deepCopyValue(elem)
		}
		return dst
	default:
		return v
	}
}

// Document is a document.Document implementation backed by map[string]any.
//
// It supports JSON Pointer (RFC 6901) paths for object navigation (maps only).
// Arrays are supported for Get navigation (e.g., "/items/0") once stored as []any.
type Document struct {
	data   map[string]any
	format document.DocumentFormat

	marshal         func(map[string]any) ([]byte, error)
	marshalTestData func(map[string]any) ([]byte, error)
}

// Ensure Document implements document.Document interface.
var _ document.Document = (*Document)(nil)

// Option configures a map-backed Document.
type Option func(*Document)

// WithData sets the initial underlying data map.
// If data is nil, it is treated as an empty map.
func WithData(data map[string]any) Option {
	return func(d *Document) {
		if data == nil {
			data = make(map[string]any)
		}
		d.data = data
	}
}

// WithMarshal sets the marshal function used by Marshal and (by default) MarshalTestData.
func WithMarshal(fn func(map[string]any) ([]byte, error)) Option {
	return func(d *Document) {
		d.marshal = fn
	}
}

// WithMarshalTestData sets the marshal function used by MarshalTestData.
func WithMarshalTestData(fn func(map[string]any) ([]byte, error)) Option {
	return func(d *Document) {
		d.marshalTestData = fn
	}
}

// New creates a new map-backed Document.
func New(format document.DocumentFormat, opts ...Option) *Document {
	d := &Document{
		data:   make(map[string]any),
		format: format,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(d)
		}
	}
	return d
}

// Data returns the underlying data map.
func (d *Document) Data() map[string]any {
	return d.data
}

// Get retrieves a value at the given JSON Pointer path.
func (d *Document) Get(path string) (any, bool) {
	if path == "" || path == "/" {
		return d.data, true
	}

	keys, err := jsonptr.Parse(path)
	if err != nil {
		return nil, false
	}
	return Get(d.data, keys)
}

// Set sets a value at the given JSON Pointer path, creating intermediate maps as needed.
func (d *Document) Set(path string, value any) error {
	if path == "" || path == "/" {
		return &document.InvalidPathError{Path: path, Reason: "cannot set root document"}
	}

	keys, err := jsonptr.Parse(path)
	if err != nil {
		return &document.InvalidPathError{Path: path, Reason: err.Error()}
	}
	if len(keys) == 0 {
		return &document.InvalidPathError{Path: path, Reason: "cannot set root document"}
	}

	Set(d.data, keys, value)
	return nil
}

// Delete removes a value at the given JSON Pointer path.
func (d *Document) Delete(path string) error {
	if path == "" || path == "/" {
		return &document.InvalidPathError{Path: path, Reason: "cannot delete root document"}
	}

	keys, err := jsonptr.Parse(path)
	if err != nil {
		return &document.InvalidPathError{Path: path, Reason: err.Error()}
	}
	if len(keys) == 0 {
		return &document.InvalidPathError{Path: path, Reason: "cannot delete root document"}
	}

	Delete(d.data, keys)
	return nil
}

// Marshal serializes the document to bytes, if configured.
func (d *Document) Marshal() ([]byte, error) {
	if d.marshal == nil {
		return nil, document.Unsupported("map-backed document does not support Marshal without a configured marshaler")
	}
	return d.marshal(d.data)
}

// Format returns the document format.
func (d *Document) Format() document.DocumentFormat {
	return d.format
}

// MarshalTestData generates bytes that, when parsed, produce a document containing the given data structure.
func (d *Document) MarshalTestData(data map[string]any) ([]byte, error) {
	if d.marshalTestData != nil {
		return d.marshalTestData(data)
	}
	if d.marshal != nil {
		return d.marshal(data)
	}
	return nil, document.Unsupported("map-backed document does not support MarshalTestData without a configured marshaler")
}
