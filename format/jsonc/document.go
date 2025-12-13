// Package jsonc provides a JSONC (JSON with comments) implementation of the
// document.Document interface.
//
// It preserves comments and formatting by operating on github.com/tailscale/hujson's AST.
// When no modifications are performed, Marshal returns the input bytes verbatim.
package jsonc

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/tailscale/hujson"
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/mapdoc"
)

// Document is a JSONC document implementation backed by hujson.Value.
type Document struct {
	root hujson.Value
}

// Ensure Document implements document.Document interface.
var _ document.Document = (*Document)(nil)

// New creates a new empty JSONC document.
func New() *Document {
	v, _ := hujson.Parse([]byte("{}"))
	return &Document{root: v}
}

// Parse parses JSONC bytes into a Document.
//
// Empty/whitespace input is treated as an empty object.
func Parse(data []byte) (document.Document, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return New(), nil
	}

	v, err := hujson.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSONC: %w", err)
	}

	if v.Value == nil {
		return New(), nil
	}
	if v.Value.Kind() != '{' {
		return nil, fmt.Errorf("failed to parse JSONC: root must be an object, got %q", v.Value.Kind())
	}

	return &Document{root: v}, nil
}

// Format returns the document format.
func (d *Document) Format() document.DocumentFormat {
	return document.FormatJSONC
}

// Marshal serializes the document to bytes while preserving comments and formatting.
func (d *Document) Marshal() ([]byte, error) {
	return d.root.Pack(), nil
}

// Get retrieves the value at the specified JSON Pointer path.
func (d *Document) Get(path string) (any, bool) {
	if path == "" || path == "/" {
		m, err := d.decodeRootToMap()
		if err != nil {
			return nil, false
		}
		return m, true
	}

	keys, err := jsonptr.Parse(path)
	if err != nil {
		return nil, false
	}

	m, err := d.decodeRootToMap()
	if err != nil {
		return nil, false
	}
	return mapdoc.Get(m, keys)
}

// Set sets the value at the specified JSON Pointer path, creating intermediate nodes as needed.
func (d *Document) Set(path string, value any) error {
	keys, err := jsonptr.Parse(path)
	if err != nil {
		return &document.InvalidPathError{Path: path, Reason: err.Error()}
	}
	if len(keys) == 0 {
		return &document.InvalidPathError{Path: path, Reason: "cannot set root document"}
	}

	// Ensure intermediate containers exist.
	for i := 0; i < len(keys)-1; i++ {
		parentPath := buildPointer(keys[:i+1])
		nextKey := keys[i+1]
		wantArray := isArrayIndex(nextKey)

		exists, kind, err := d.peekKind(parentPath)
		if err != nil {
			return err
		}

		switch {
		case !exists:
			if wantArray {
				if err := d.patchAdd(parentPath, []any{}); err != nil {
					return err
				}
			} else {
				if err := d.patchAdd(parentPath, map[string]any{}); err != nil {
					return err
				}
			}
		case wantArray && kind != '[':
			if err := d.patchAdd(parentPath, []any{}); err != nil {
				return err
			}
		case !wantArray && kind != '{':
			if err := d.patchAdd(parentPath, map[string]any{}); err != nil {
				return err
			}
		}

		if wantArray {
			idx, _ := parseArrayIndex(nextKey)
			if err := d.ensureArrayLen(parentPath, idx+1); err != nil {
				return err
			}
		}
	}

	// Apply final set.
	exists, _, err := d.peekKind(path)
	if err != nil {
		return err
	}
	if exists {
		return d.patchReplace(path, value)
	}
	return d.patchAdd(path, value)
}

// Delete removes the value at the specified JSON Pointer path.
func (d *Document) Delete(path string) error {
	keys, err := jsonptr.Parse(path)
	if err != nil {
		return &document.InvalidPathError{Path: path, Reason: err.Error()}
	}
	if len(keys) == 0 {
		return &document.InvalidPathError{Path: path, Reason: "cannot delete root document"}
	}

	exists, _, err := d.peekKind(path)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	return d.patchRemove(path)
}

// MarshalTestData generates JSON bytes (without comments) for testing.
func (d *Document) MarshalTestData(data map[string]any) ([]byte, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSONC test data: %w", err)
	}
	return append(b, '\n'), nil
}

func (d *Document) decodeRootToMap() (map[string]any, error) {
	v := d.root.Clone()
	v.Standardize()

	var root any
	if err := json.Unmarshal(v.Pack(), &root); err != nil {
		return nil, fmt.Errorf("failed to decode JSONC: %w", err)
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("failed to decode JSONC: root must be an object, got %T", root)
	}
	return obj, nil
}

func (d *Document) peekKind(path string) (exists bool, kind hujson.Kind, err error) {
	m, err := d.decodeRootToMap()
	if err != nil {
		return false, 0, err
	}
	if path == "" || path == "/" {
		return true, '{', nil
	}
	keys, err := jsonptr.Parse(path)
	if err != nil {
		return false, 0, &document.InvalidPathError{Path: path, Reason: err.Error()}
	}
	v, ok := mapdoc.Get(m, keys)
	if !ok {
		return false, 0, nil
	}
	switch v.(type) {
	case map[string]any:
		return true, '{', nil
	case []any:
		return true, '[', nil
	case nil:
		return true, 'n', nil
	case bool:
		if v.(bool) {
			return true, 't', nil
		}
		return true, 'f', nil
	case string:
		return true, '"', nil
	default:
		// encoding/json decodes numbers as float64
		return true, '0', nil
	}
}

func (d *Document) ensureArrayLen(arrayPath string, want int) error {
	exists, kind, err := d.peekKind(arrayPath)
	if err != nil {
		return err
	}
	if !exists || kind != '[' {
		return fmt.Errorf("expected array at %q", arrayPath)
	}

	m, err := d.decodeRootToMap()
	if err != nil {
		return err
	}
	ptrKeys, _ := jsonptr.Parse(arrayPath)
	curAny, _ := mapdoc.Get(m, ptrKeys)
	cur, _ := curAny.([]any)

	for len(cur) < want {
		if err := d.patchAdd(arrayPath+"/-", nil); err != nil {
			return err
		}
		cur = append(cur, nil)
	}
	return nil
}

func buildPointer(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	var b bytes.Buffer
	for _, k := range keys {
		b.WriteByte('/')
		b.WriteString(jsonptr.Escape(k))
	}
	return b.String()
}

func (d *Document) patchAdd(path string, value any) error {
	return d.patch("add", path, value)
}

func (d *Document) patchReplace(path string, value any) error {
	return d.patch("replace", path, value)
}

func (d *Document) patchRemove(path string) error {
	return d.patch("remove", path, nil)
}

func (d *Document) patch(op string, path string, value any) error {
	patch := map[string]any{
		"op":   op,
		"path": path,
	}
	if op == "add" || op == "replace" {
		patch["value"] = value
	}
	b, err := json.Marshal([]any{patch})
	if err != nil {
		return fmt.Errorf("failed to build JSONC patch: %w", err)
	}
	if err := d.root.Patch(b); err != nil {
		return fmt.Errorf("failed to apply JSONC patch: %w", err)
	}
	return nil
}

func isArrayIndex(s string) bool {
	_, err := parseArrayIndex(s)
	return err == nil
}

func parseArrayIndex(s string) (int, error) {
	// Reuse mapdoc's parsing logic by mirroring constraints:
	// numeric string, non-empty, non-negative.
	if s == "" {
		return 0, fmt.Errorf("array index must be non-empty")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("array index must be numeric: %q", s)
		}
	}
	var n int
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n, nil
}
