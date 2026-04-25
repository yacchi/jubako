package coordinated

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
)

// DecodeAt decodes the value at path into T.
func DecodeAt[T any](c any, raw map[string]any, path string) (T, error) {
	return decodeValueAt[T](raw, path)
}

// BeforeAt decodes the "before save" value at path into T.
func BeforeAt[T any](c SaveContext, path string) (T, bool, error) {
	return decodeOptionalAt[T](c.Logical(), path)
}

// AfterAt decodes the "after save" value at path into T.
func AfterAt[T any](c SaveContext, changes document.JSONPatchSet, path string) (T, bool, error) {
	raw, err := c.LogicalAfter(changes)
	if err != nil {
		var zero T
		return zero, false, err
	}
	return decodeOptionalAt[T](raw, path)
}

// TaggedDescendants returns all concrete descendant descriptors under root that
// match the given tag key/value pair.
func TaggedDescendants(schema SchemaView, root, tagKey, tagValue string) ([]PathDescriptor, error) {
	rootSegments, err := jsonptr.Parse(root)
	if err != nil {
		return nil, err
	}

	var out []PathDescriptor
	for _, desc := range schema.Descriptors() {
		value, ok := desc.Tag(tagKey)
		if !ok || value != tagValue {
			continue
		}

		concretePath, ok, err := concretizeDescriptorPath(desc.Path(), rootSegments)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		out = append(out, taggedDescriptor{
			PathDescriptor: desc,
			path:           concretePath,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Path() < out[j].Path()
	})
	return out, nil
}

// FilterPatches returns the subset of patches that match the predicate.
func FilterPatches(changes document.JSONPatchSet, keep func(path string) bool) document.JSONPatchSet {
	filtered := make(document.JSONPatchSet, 0, len(changes))
	for _, patch := range changes {
		if keep(patch.Path) {
			filtered = append(filtered, patch)
		}
	}
	return filtered
}

type taggedDescriptor struct {
	PathDescriptor
	path string
}

func (d taggedDescriptor) Path() string {
	return d.path
}

func concretizeDescriptorPath(path string, rootSegments []string) (string, bool, error) {
	segments, err := jsonptr.Parse(path)
	if err != nil {
		return "", false, err
	}
	if len(segments) < len(rootSegments) {
		return "", false, nil
	}

	for i, rootSegment := range rootSegments {
		segment := segments[i]
		if segment != "*" && segment != rootSegment {
			return "", false, nil
		}
	}

	remainder := segments[len(rootSegments):]
	for _, segment := range remainder {
		if segment == "*" {
			return "", false, nil
		}
	}

	concrete := append(append([]string(nil), rootSegments...), remainder...)
	return jsonptr.Build(stringSliceToAny(concrete)...), true, nil
}

func stringSliceToAny(in []string) []any {
	out := make([]any, len(in))
	for i, value := range in {
		out[i] = value
	}
	return out
}

func decodeOptionalAt[T any](raw map[string]any, path string) (T, bool, error) {
	var zero T
	if path == "" {
		value, err := decodeValue[T](raw)
		return value, true, err
	}
	if _, ok := jsonptr.GetPath(raw, path); !ok {
		return zero, false, nil
	}
	value, err := decodeValueAt[T](raw, path)
	return value, true, err
}

func decodeValueAt[T any](raw map[string]any, path string) (T, error) {
	if path == "" {
		return decodeValue[T](raw)
	}

	value, ok := jsonptr.GetPath(raw, path)
	if !ok {
		var zero T
		return zero, fmt.Errorf("path %q not found", path)
	}
	return decodeValue[T](value)
}

func decodeValue[T any](value any) (T, error) {
	var out T
	data, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

// HasPathPrefix reports whether path is the given root or a descendant of it.
func HasPathPrefix(path string, root string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}
