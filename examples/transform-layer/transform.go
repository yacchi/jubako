package main

import (
	"context"
	"strings"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/types"
)

// PathMapping defines a bidirectional path transformation.
type PathMapping struct {
	// Canonical is the path used by the application (v2 format)
	Canonical string
	// Source is the path in the underlying document (v1 format)
	Source string
}

// TransformLayer wraps a Layer and transforms paths bidirectionally.
// This enables reading/writing with a canonical schema while the
// underlying document uses a different structure.
type TransformLayer struct {
	inner    layer.Layer
	mappings []PathMapping
	// Pre-built lookup maps for efficient transformation
	canonicalToSource map[string]string
	sourceToCanonical map[string]string
}

// NewTransformLayer creates a new TransformLayer that wraps the given layer.
func NewTransformLayer(inner layer.Layer, mappings []PathMapping) *TransformLayer {
	t := &TransformLayer{
		inner:             inner,
		mappings:          mappings,
		canonicalToSource: make(map[string]string),
		sourceToCanonical: make(map[string]string),
	}
	for _, m := range mappings {
		t.canonicalToSource[m.Canonical] = m.Source
		t.sourceToCanonical[m.Source] = m.Canonical
	}
	return t
}

func (t *TransformLayer) Name() layer.Name {
	return t.inner.Name()
}

// Load reads from the underlying layer and transforms paths to canonical format.
func (t *TransformLayer) Load(ctx context.Context) (map[string]any, error) {
	data, err := t.inner.Load(ctx)
	if err != nil {
		return nil, err
	}
	return t.transformToCanonical(data), nil
}

// Save transforms paths back to source format and saves to the underlying layer.
func (t *TransformLayer) Save(ctx context.Context, changeset document.JSONPatchSet) error {
	// Transform changeset paths back to source format.
	sourceChangeset := make(document.JSONPatchSet, len(changeset))
	for i, patch := range changeset {
		sourceChangeset[i] = document.JSONPatch{
			Op:    patch.Op,
			Path:  t.toSourcePath(patch.Path),
			Value: patch.Value,
		}
	}

	return t.inner.Save(ctx, sourceChangeset)
}

func (t *TransformLayer) CanSave() bool {
	return t.inner.CanSave()
}

// FillDetails delegates to the inner layer's FillDetails method.
func (t *TransformLayer) FillDetails(d *types.Details) {
	t.inner.FillDetails(d)
}

// Watch delegates to the inner layer's Watch method.
func (t *TransformLayer) Watch(opts ...layer.WatchOption) (layer.LayerWatcher, error) {
	return t.inner.Watch(opts...)
}

// transformToCanonical transforms a map from source format to canonical format.
func (t *TransformLayer) transformToCanonical(src map[string]any) map[string]any {
	result := make(map[string]any)

	// First, apply explicit mappings
	for canonical, source := range t.canonicalToSource {
		value, ok := jsonptr.GetPath(src, source)
		if !ok {
			continue
		}
		jsonptr.SetPath(result, canonical, value)
	}

	// Then, copy unmapped values (passthrough)
	t.copyUnmappedValues(result, src, "")

	return result
}

// transformToSource transforms a map from canonical format to source format.
func (t *TransformLayer) transformToSource(canonical map[string]any) map[string]any {
	result := make(map[string]any)

	// First, apply explicit mappings in reverse
	for canonicalPath, sourcePath := range t.canonicalToSource {
		value, ok := jsonptr.GetPath(canonical, canonicalPath)
		if !ok {
			continue
		}
		jsonptr.SetPath(result, sourcePath, value)
	}

	// Then, copy unmapped values (passthrough)
	t.copyUnmappedValuesToSource(result, canonical, "")

	return result
}

// copyUnmappedValues copies values that don't have explicit mappings.
func (t *TransformLayer) copyUnmappedValues(dst map[string]any, src map[string]any, prefix string) {
	for key, value := range src {
		srcPath := prefix + "/" + jsonptr.Escape(key)

		// Check if this path or any child has a mapping
		if t.hasMappingForSource(srcPath) {
			// If it's a map, recurse to check children
			if nested, ok := value.(map[string]any); ok {
				t.copyUnmappedValues(dst, nested, srcPath)
			}
			continue
		}

		// No mapping for this path - copy as-is to canonical structure
		canonicalPath := t.toCanonicalPath(srcPath)
		jsonptr.SetPath(dst, canonicalPath, value)
	}
}

// copyUnmappedValuesToSource copies values that don't have explicit mappings (reverse).
func (t *TransformLayer) copyUnmappedValuesToSource(dst map[string]any, src map[string]any, prefix string) {
	for key, value := range src {
		canonicalPath := prefix + "/" + jsonptr.Escape(key)

		// Check if this path or any child has a mapping
		if t.hasMappingForCanonical(canonicalPath) {
			// If it's a map, recurse to check children
			if nested, ok := value.(map[string]any); ok {
				t.copyUnmappedValuesToSource(dst, nested, canonicalPath)
			}
			continue
		}

		// No mapping for this path - copy as-is to source structure
		sourcePath := t.toSourcePath(canonicalPath)
		jsonptr.SetPath(dst, sourcePath, value)
	}
}

// toSourcePath converts a canonical path to the source path.
func (t *TransformLayer) toSourcePath(canonicalPath string) string {
	if sourcePath, ok := t.canonicalToSource[canonicalPath]; ok {
		return sourcePath
	}
	return canonicalPath // No mapping, use as-is
}

// toCanonicalPath converts a source path to the canonical path.
func (t *TransformLayer) toCanonicalPath(sourcePath string) string {
	if canonicalPath, ok := t.sourceToCanonical[sourcePath]; ok {
		return canonicalPath
	}
	return sourcePath // No mapping, use as-is
}

// hasMappingForSource checks if a source path has any mapping (exact or prefix match).
func (t *TransformLayer) hasMappingForSource(sourcePath string) bool {
	for _, src := range t.canonicalToSource {
		if src == sourcePath || strings.HasPrefix(src, sourcePath+"/") {
			return true
		}
	}
	return false
}

// hasMappingForCanonical checks if a canonical path has any mapping (exact or prefix match).
func (t *TransformLayer) hasMappingForCanonical(canonicalPath string) bool {
	for canonical := range t.canonicalToSource {
		if canonical == canonicalPath || strings.HasPrefix(canonical, canonicalPath+"/") {
			return true
		}
	}
	return false
}
