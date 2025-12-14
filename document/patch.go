// Package document provides the Document interface and related types.
package document

import "github.com/yacchi/jubako/jsonptr"

// PatchOp represents a JSON Patch operation type (RFC 6902).
type PatchOp string

const (
	// PatchOpAdd adds a value at the target location.
	PatchOpAdd PatchOp = "add"
	// PatchOpRemove removes the value at the target location.
	PatchOpRemove PatchOp = "remove"
	// PatchOpReplace replaces the value at the target location.
	PatchOpReplace PatchOp = "replace"
)

// JSONPatch represents a single JSON Patch operation (RFC 6902).
type JSONPatch struct {
	// Op is the operation type.
	Op PatchOp `json:"op"`
	// Path is the JSON Pointer (RFC 6901) to the target location.
	Path string `json:"path"`
	// Value is the value for "add" and "replace" operations.
	Value any `json:"value,omitempty"`
}

// NewAddPatch creates an "add" patch operation.
func NewAddPatch(path string, value any) JSONPatch {
	return JSONPatch{Op: PatchOpAdd, Path: path, Value: value}
}

// NewRemovePatch creates a "remove" patch operation.
func NewRemovePatch(path string) JSONPatch {
	return JSONPatch{Op: PatchOpRemove, Path: path}
}

// NewReplacePatch creates a "replace" patch operation.
func NewReplacePatch(path string, value any) JSONPatch {
	return JSONPatch{Op: PatchOpReplace, Path: path, Value: value}
}

// JSONPatchSet is a collection of JSON Patch operations.
// It provides a type-safe way to pass patch operations to Document.Apply().
type JSONPatchSet []JSONPatch

// Add appends an "add" operation to the patch set.
func (ps *JSONPatchSet) Add(path string, value any) {
	*ps = append(*ps, NewAddPatch(path, value))
}

// Remove appends a "remove" operation to the patch set.
func (ps *JSONPatchSet) Remove(path string) {
	*ps = append(*ps, NewRemovePatch(path))
}

// Replace appends a "replace" operation to the patch set.
func (ps *JSONPatchSet) Replace(path string, value any) {
	*ps = append(*ps, NewReplacePatch(path, value))
}

// Len returns the number of patches in the set.
func (ps JSONPatchSet) Len() int {
	return len(ps)
}

// IsEmpty returns true if the patch set contains no operations.
func (ps JSONPatchSet) IsEmpty() bool {
	return len(ps) == 0
}

// ApplyTo applies all patch operations to the given map.
// Invalid paths are silently skipped.
func (ps JSONPatchSet) ApplyTo(data map[string]any) {
	for _, patch := range ps {
		switch patch.Op {
		case PatchOpAdd, PatchOpReplace:
			jsonptr.SetPath(data, patch.Path, patch.Value)
		case PatchOpRemove:
			jsonptr.DeletePath(data, patch.Path)
		}
	}
}
