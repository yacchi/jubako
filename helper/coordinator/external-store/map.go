package externalstore

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/yacchi/jubako/container"
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/layer/coordinated"
)

// Route describes how a logical entry maps to metadata and the external store.
type Route struct {
	UseExternal bool
	Ref         string
	// ExternalKey optionally overrides the physical backend key. When empty,
	// the helper uses Ref as the external key.
	ExternalKey string
}

// RouteContext provides the input required to derive routing for a logical map entry.
// When ExistingRef is non-empty, RouteForEntry must keep Ref stable. ExternalKey
// may be omitted, in which case the helper uses Ref as the backend key, even if
// UseExternal is false, so the helper can:
//   - hydrate the current logical value from existing external state
//   - clean up old external state when reprojection moves the entry back to local storage
//
// Entry exposes the typed logical entry rooted at EntryPath. Logical remains available
// for route decisions that also depend on sibling or global config outside the entry.
type RouteContext[T any] struct {
	Key         string
	EntryPath   string
	ExistingRef string
	Logical     map[string]any
	Entry       T
	HasEntry    bool
}

// RouteFunc derives whether an entry should use the external store and, when
// needed, the stable document-side ref plus optional physical external-store key override.
//
// Route selection describes the target projection, not whether current external
// state exists. When ExistingRef is non-empty, the helper may still hydrate from
// ExternalKey (or Ref when ExternalKey is empty) before reprojecting the entry
// to its target destination.
type RouteFunc[T any] func(RouteContext[T]) (Route, error)

// ExternalContext provides reference information for an external-store operation.
// The payload persisted to the external store remains a minimal map[string]any,
// while Before/After expose the logical typed entry for backend-specific metadata
// decisions such as labels, comments, or expiry.
type ExternalContext[T any] struct {
	Key         string
	Ref         string
	ExternalKey string
	EntryPath   string
	Before      T
	After       T
	HasBefore   bool
	HasAfter    bool
}

// SecretStore stores the externalized portion of a logical subtree.
//
// Get should return an error matching ErrNotExist when the external entry does
// not exist.
type SecretStore[T any] interface {
	Get(ctx context.Context, c ExternalContext[T]) (map[string]any, error)
	Set(ctx context.Context, c ExternalContext[T], value map[string]any) error
	Delete(ctx context.Context, c ExternalContext[T]) error
}

// MapConfig configures a map-root external store projection.
type MapConfig[T any] struct {
	RootPath         string
	Metadata         layer.Layer
	External         SecretStore[T]
	RefPath          string
	ExternalTagKey   string
	ExternalTagValue string
	RouteForEntry    RouteFunc[T]
}

// NewMap creates a coordinated layer that externalizes tagged fields for map entries under RootPath.
func NewMap[T any](name layer.Name, cfg MapConfig[T]) (layer.Layer, error) {
	coordinator, err := newMapCoordinator[T](cfg)
	if err != nil {
		return nil, err
	}
	return coordinated.New(name, coordinator), nil
}

type mapCoordinator[T any] struct {
	rootPath         string
	rootSegments     []string
	metadata         layer.Layer
	external         SecretStore[T]
	refPath          string
	externalTagKey   string
	externalTagValue string
	routeForEntry    RouteFunc[T]
	loaded           map[string]any
}

func newMapCoordinator[T any](cfg MapConfig[T]) (*mapCoordinator[T], error) {
	if cfg.RootPath == "" {
		return nil, fmt.Errorf("external-store: RootPath is required")
	}
	if cfg.Metadata == nil {
		return nil, fmt.Errorf("external-store: Metadata is required")
	}
	if !cfg.Metadata.CanSave() {
		return nil, fmt.Errorf("external-store: Metadata layer %q must support saving", cfg.Metadata.Name())
	}
	if cfg.External == nil {
		return nil, fmt.Errorf("external-store: External store is required")
	}
	if cfg.RouteForEntry == nil {
		return nil, fmt.Errorf("external-store: RouteForEntry is required")
	}

	rootSegments, err := jsonptr.Parse(cfg.RootPath)
	if err != nil || len(rootSegments) == 0 {
		return nil, fmt.Errorf("external-store: invalid RootPath %q", cfg.RootPath)
	}

	refPath := cfg.RefPath
	if refPath == "" {
		refPath = "/secret_ref"
	}
	if !strings.HasPrefix(refPath, "/") {
		refPath = "/" + refPath
	}

	tagKey := cfg.ExternalTagKey
	if tagKey == "" {
		tagKey = "storage"
	}
	tagValue := cfg.ExternalTagValue
	if tagValue == "" {
		tagValue = "external"
	}

	return &mapCoordinator[T]{
		rootPath:         cfg.RootPath,
		rootSegments:     rootSegments,
		metadata:         cfg.Metadata,
		external:         cfg.External,
		refPath:          refPath,
		externalTagKey:   tagKey,
		externalTagValue: tagValue,
		routeForEntry:    cfg.RouteForEntry,
	}, nil
}

func (c *mapCoordinator[T]) Load(ctx context.Context, cc coordinated.LoadContext) (map[string]any, error) {
	metadata, err := c.metadata.Load(ctx)
	if err != nil {
		return nil, err
	}
	c.loaded = container.DeepCopyMap(metadata)
	return metadata, nil
}

func (c *mapCoordinator[T]) Stabilize(ctx context.Context, cc coordinated.StabilizeContext) (*coordinated.StabilizeResult, error) {
	snapshot := cc.Snapshot()

	base := c.loaded
	if base == nil {
		base = make(map[string]any)
	}
	data := container.DeepCopyMap(base)
	if currentRoot, ok := jsonptr.GetPath(snapshot, c.rootPath); ok {
		if result := jsonptr.SetPath(data, c.rootPath, container.DeepCopyValue(currentRoot)); !result.Success {
			return nil, fmt.Errorf("external-store: failed to prepare subtree %q", c.rootPath)
		}
	}
	current := mapEntriesAt(data, c.rootPath)
	if current == nil {
		return &coordinated.StabilizeResult{}, nil
	}

	changed := false
	dirty := make([]string, 0)
	for key, raw := range current {
		entryValue, err := coordinated.DecodeAt[T](cc, map[string]any{"value": raw}, "/value")
		if err != nil {
			return nil, err
		}

		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("external-store: entry %q at %q is %T, want map[string]any", key, c.entryPath(key), raw)
		}

		ref, _ := stringAt(entry, c.refPath)
		route, err := c.resolveRoute(RouteContext[T]{
			Key:         key,
			EntryPath:   c.entryPath(key),
			ExistingRef: ref,
			Logical:     snapshot,
			Entry:       entryValue,
			HasEntry:    true,
		})
		if err != nil {
			return nil, err
		}
		if ref != "" {
			payload, err := c.external.Get(ctx, ExternalContext[T]{
				Key:         key,
				Ref:         ref,
				ExternalKey: route.ExternalKey,
				EntryPath:   c.entryPath(key),
				After:       entryValue,
				HasAfter:    true,
			})
			if err != nil {
				if !errors.Is(err, ErrNotExist) {
					return nil, err
				}
				payload = nil
			}
			entryChanged, err := c.hydrateTaggedFields(cc.Schema(), c.entryPath(key), entry, payload)
			if err != nil {
				return nil, err
			}
			if entryChanged {
				changed = true
			}
		}

		tagged, err := c.containsTaggedFields(cc.Schema(), c.entryPath(key), entry)
		if err != nil {
			return nil, err
		}
		switch {
		case ref == "" && route.UseExternal && tagged:
			dirty = append(dirty, c.entryPath(key))
		case ref != "" && !route.UseExternal:
			dirty = append(dirty, c.entryPath(key))
		}
	}

	return &coordinated.StabilizeResult{
		Data:            data,
		Changed:         changed,
		ProjectionDirty: dirty,
	}, nil
}

func (c *mapCoordinator[T]) Save(ctx context.Context, cc coordinated.SaveContext, changes document.JSONPatchSet) error {
	beforeLogical := cc.Logical()
	afterLogical, err := cc.LogicalAfter(changes)
	if err != nil {
		return err
	}

	for _, key := range c.changedKeys(beforeLogical, afterLogical, changes) {
		beforeRaw, beforeExists, err := entryValueAt(beforeLogical, c.entryPath(key))
		if err != nil {
			return err
		}
		afterRaw, afterExists, err := entryValueAt(afterLogical, c.entryPath(key))
		if err != nil {
			return err
		}

		var beforeValue T
		var afterValue T
		if beforeExists {
			beforeValue, err = coordinated.DecodeAt[T](cc, map[string]any{"value": beforeRaw}, "/value")
			if err != nil {
				return err
			}
		}
		if afterExists {
			afterValue, err = coordinated.DecodeAt[T](cc, map[string]any{"value": afterRaw}, "/value")
			if err != nil {
				return err
			}
		}

		existingRef, _ := stringAt(beforeRaw, c.refPath)

		if !afterExists {
			if existingRef != "" {
				beforeRoute, err := c.resolveRoute(RouteContext[T]{
					Key:         key,
					EntryPath:   c.entryPath(key),
					ExistingRef: existingRef,
					Logical:     beforeLogical,
					Entry:       beforeValue,
					HasEntry:    beforeExists,
				})
				if err != nil {
					return err
				}
				if beforeRoute.ExternalKey != "" {
					if err := c.external.Delete(ctx, ExternalContext[T]{
						Key:         key,
						Ref:         existingRef,
						ExternalKey: beforeRoute.ExternalKey,
						EntryPath:   c.entryPath(key),
						Before:      beforeValue,
						HasBefore:   beforeExists,
					}); err != nil {
						return err
					}
				}
			}
			if err := c.writeMetadata(ctx, c.entryPath(key), nil); err != nil {
				return err
			}
			continue
		}

		afterRoute, err := c.resolveRoute(RouteContext[T]{
			Key:         key,
			EntryPath:   c.entryPath(key),
			ExistingRef: existingRef,
			Logical:     afterLogical,
			Entry:       afterValue,
			HasEntry:    afterExists,
		})
		if err != nil {
			return err
		}

		if afterRoute.UseExternal {
			metadata, secret, err := c.splitTaggedFields(cc.Schema(), c.entryPath(key), afterRaw)
			if err != nil {
				return err
			}
			if err := c.external.Set(ctx, ExternalContext[T]{
				Key:         key,
				Ref:         afterRoute.Ref,
				ExternalKey: afterRoute.ExternalKey,
				EntryPath:   c.entryPath(key),
				Before:      beforeValue,
				After:       afterValue,
				HasBefore:   beforeExists,
				HasAfter:    afterExists,
			}, secret); err != nil {
				return err
			}
			if existingRef != "" {
				beforeRoute, err := c.resolveRoute(RouteContext[T]{
					Key:         key,
					EntryPath:   c.entryPath(key),
					ExistingRef: existingRef,
					Logical:     beforeLogical,
					Entry:       beforeValue,
					HasEntry:    beforeExists,
				})
				if err != nil {
					return err
				}
				if beforeRoute.ExternalKey != afterRoute.ExternalKey {
					if err := c.external.Delete(ctx, ExternalContext[T]{
						Key:         key,
						Ref:         existingRef,
						ExternalKey: beforeRoute.ExternalKey,
						EntryPath:   c.entryPath(key),
						Before:      beforeValue,
						After:       afterValue,
						HasBefore:   beforeExists,
						HasAfter:    afterExists,
					}); err != nil {
						return err
					}
				}
			}
			if result := jsonptr.SetPath(metadata, c.refPath, afterRoute.Ref); !result.Success {
				return fmt.Errorf("external-store: failed to set ref at %q", c.refPath)
			}
			if err := c.writeMetadata(ctx, c.entryPath(key), metadata); err != nil {
				return err
			}
			continue
		}

		metadata := container.DeepCopyMap(afterRaw)
		if existingRef != "" {
			beforeRoute, err := c.resolveRoute(RouteContext[T]{
				Key:         key,
				EntryPath:   c.entryPath(key),
				ExistingRef: existingRef,
				Logical:     beforeLogical,
				Entry:       beforeValue,
				HasEntry:    beforeExists,
			})
			if err != nil {
				return err
			}
			if beforeRoute.ExternalKey != "" {
				if err := c.external.Delete(ctx, ExternalContext[T]{
					Key:         key,
					Ref:         existingRef,
					ExternalKey: beforeRoute.ExternalKey,
					EntryPath:   c.entryPath(key),
					Before:      beforeValue,
					After:       afterValue,
					HasBefore:   beforeExists,
					HasAfter:    afterExists,
				}); err != nil {
					return err
				}
			}
		}
		jsonptr.DeletePath(metadata, c.refPath)
		if err := c.writeMetadata(ctx, c.entryPath(key), metadata); err != nil {
			return err
		}
	}

	return nil
}

func (c *mapCoordinator[T]) changedKeys(before, after map[string]any, changes document.JSONPatchSet) []string {
	seen := make(map[string]struct{})
	keys := make([]string, 0)

	addKey := func(key string) {
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}

	addAll := func(values map[string]any) {
		for key := range values {
			addKey(key)
		}
	}

	for _, patch := range changes {
		segments, err := jsonptr.Parse(patch.Path)
		if err != nil || !hasPrefixSegments(segments, c.rootSegments) {
			continue
		}
		if len(segments) == len(c.rootSegments) {
			addAll(mapEntriesAt(before, c.rootPath))
			addAll(mapEntriesAt(after, c.rootPath))
			continue
		}
		addKey(segments[len(c.rootSegments)])
	}

	sort.Strings(keys)
	return keys
}

func (c *mapCoordinator[T]) entryPath(key string) string {
	return c.rootPath + "/" + jsonptr.Escape(key)
}

func (c *mapCoordinator[T]) resolveRoute(ctx RouteContext[T]) (Route, error) {
	route, err := c.routeForEntry(ctx)
	if err != nil {
		return Route{}, err
	}
	if ctx.ExistingRef != "" {
		if route.Ref != ctx.ExistingRef {
			return Route{}, fmt.Errorf(
				"external-store: RouteForEntry changed existing ref for entry %q: got %q want %q",
				ctx.Key, route.Ref, ctx.ExistingRef,
			)
		}
	}
	if route.UseExternal && route.Ref == "" {
		return Route{}, fmt.Errorf("external-store: RouteForEntry returned empty ref for external entry %q", ctx.Key)
	}
	if route.ExternalKey == "" && route.Ref != "" {
		route.ExternalKey = route.Ref
	}
	return route, nil
}

func (c *mapCoordinator[T]) hydrateTaggedFields(
	schema coordinated.SchemaView,
	entryPath string,
	entry map[string]any,
	payload map[string]any,
) (bool, error) {
	fields, err := coordinated.TaggedDescendants(schema, entryPath, c.externalTagKey, c.externalTagValue)
	if err != nil {
		return false, err
	}
	changed := false
	for _, field := range fields {
		rel := strings.TrimPrefix(field.Path(), entryPath)
		value, ok := jsonptr.GetPath(payload, rel)
		if !ok {
			continue
		}
		current, exists := jsonptr.GetPath(entry, rel)
		if exists && reflect.DeepEqual(current, value) {
			continue
		}
		if result := jsonptr.SetPath(entry, rel, value); !result.Success {
			return false, fmt.Errorf("external-store: failed to hydrate %q", rel)
		}
		changed = true
	}
	return changed, nil
}

func (c *mapCoordinator[T]) splitTaggedFields(
	schema coordinated.SchemaView,
	entryPath string,
	entry map[string]any,
) (map[string]any, map[string]any, error) {
	metadata := container.DeepCopyMap(entry)
	secret := make(map[string]any)

	fields, err := coordinated.TaggedDescendants(schema, entryPath, c.externalTagKey, c.externalTagValue)
	if err != nil {
		return nil, nil, err
	}
	for _, field := range fields {
		rel := strings.TrimPrefix(field.Path(), entryPath)
		value, ok := jsonptr.GetPath(metadata, rel)
		if !ok {
			continue
		}
		if result := jsonptr.SetPath(secret, rel, value); !result.Success {
			return nil, nil, fmt.Errorf("external-store: failed to set secret %q", rel)
		}
		if deleted := jsonptr.DeletePath(metadata, rel); !deleted {
			return nil, nil, fmt.Errorf("external-store: failed to delete metadata %q", rel)
		}
	}

	return metadata, secret, nil
}

func (c *mapCoordinator[T]) containsTaggedFields(
	schema coordinated.SchemaView,
	entryPath string,
	entry map[string]any,
) (bool, error) {
	fields, err := coordinated.TaggedDescendants(schema, entryPath, c.externalTagKey, c.externalTagValue)
	if err != nil {
		return false, err
	}
	for _, field := range fields {
		rel := strings.TrimPrefix(field.Path(), entryPath)
		if _, ok := jsonptr.GetPath(entry, rel); ok {
			return true, nil
		}
	}
	return false, nil
}

func (c *mapCoordinator[T]) writeMetadata(ctx context.Context, entryPath string, metadata map[string]any) error {
	current, err := c.metadata.Load(ctx)
	if err != nil {
		return err
	}

	var patches document.JSONPatchSet
	if metadata == nil {
		if jsonptr.DeletePath(current, entryPath) {
			patches.Remove(entryPath)
		}
	} else {
		_, existed := jsonptr.GetPath(current, entryPath)
		if result := jsonptr.SetPath(current, entryPath, metadata); !result.Success {
			return fmt.Errorf("external-store: failed to set metadata for %q", entryPath)
		}
		if existed {
			patches.Replace(entryPath, metadata)
		} else {
			patches.Add(entryPath, metadata)
		}
	}

	if patches.IsEmpty() {
		return nil
	}
	if err := c.metadata.Save(ctx, patches); err != nil {
		return err
	}
	c.loaded = current
	return nil
}

func mapEntriesAt(data map[string]any, root string) map[string]any {
	value, ok := jsonptr.GetPath(data, root)
	if !ok {
		return nil
	}
	entries, _ := value.(map[string]any)
	return entries
}

func entryValueAt(data map[string]any, path string) (map[string]any, bool, error) {
	value, ok := jsonptr.GetPath(data, path)
	if !ok {
		return nil, false, nil
	}
	entry, ok := value.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("external-store: value at %q is %T, want map[string]any", path, value)
	}
	return entry, true, nil
}

func stringAt(data map[string]any, path string) (string, bool) {
	if data == nil {
		return "", false
	}
	value, ok := jsonptr.GetPath(data, path)
	if !ok {
		return "", false
	}
	s, ok := value.(string)
	return s, ok
}

func hasPrefixSegments(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i, segment := range prefix {
		if path[i] != segment {
			return false
		}
	}
	return true
}
