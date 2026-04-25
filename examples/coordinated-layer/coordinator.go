package main

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/yacchi/jubako/container"
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/layer/coordinated"
	"github.com/yacchi/jubako/layer/mapdata"
)

const (
	refPath          = "/secret_ref"
	externalTagKey   = "storage"
	externalTagValue = "keyring"
)

type memorySecretStore struct {
	values map[string]map[string]any
}

func newMemorySecretStore() *memorySecretStore {
	return &memorySecretStore{
		values: make(map[string]map[string]any),
	}
}

func (s *memorySecretStore) Get(key string) (map[string]any, bool) {
	value, ok := s.values[key]
	if !ok {
		return nil, false
	}
	return container.DeepCopyMap(value), true
}

func (s *memorySecretStore) Set(key string, value map[string]any) {
	s.values[key] = container.DeepCopyMap(value)
}

func (s *memorySecretStore) Delete(key string) {
	delete(s.values, key)
}

func (s *memorySecretStore) Snapshot() map[string]map[string]any {
	out := make(map[string]map[string]any, len(s.values))
	for key, value := range s.values {
		out[key] = container.DeepCopyMap(value)
	}
	return out
}

type credentialsCoordinator struct {
	metadata layer.Layer
	secrets  *memorySecretStore
	loaded   map[string]any
}

func newCredentialsLayer(metadata layer.Layer, secrets *memorySecretStore) (layer.Layer, error) {
	if metadata == nil {
		return nil, fmt.Errorf("metadata layer is required")
	}
	if !metadata.CanSave() {
		return nil, fmt.Errorf("metadata layer %q must support saving", metadata.Name())
	}
	if secrets == nil {
		return nil, fmt.Errorf("secret store is required")
	}
	return coordinated.New(layerCredentials, &credentialsCoordinator{
		metadata: metadata,
		secrets:  secrets,
	}), nil
}

func newCredentialsMetadata(data map[string]any) *mapdata.Layer {
	return mapdata.New("credentials-metadata", data)
}

func (c *credentialsCoordinator) Load(ctx context.Context, _ coordinated.LoadContext) (map[string]any, error) {
	metadata, err := c.metadata.Load(ctx)
	if err != nil {
		return nil, err
	}
	c.loaded = container.DeepCopyMap(metadata)
	return metadata, nil
}

func (c *credentialsCoordinator) Stabilize(ctx context.Context, sc coordinated.StabilizeContext) (*coordinated.StabilizeResult, error) {
	snapshot := sc.Snapshot()
	data := c.baseData(snapshot)

	current := mapEntriesAt(data, pathCredential)
	if current == nil {
		return &coordinated.StabilizeResult{}, nil
	}

	changed := false
	dirty := make([]string, 0)
	for key, raw := range current {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("credential entry %q is %T, want map[string]any", key, raw)
		}

		ref, _ := stringAt(entry, refPath)
		if ref != "" {
			payload, ok := c.secrets.Get(ref)
			if ok {
				entryChanged, err := hydrateTaggedFields(sc.Schema(), entryPath(key), entry, payload)
				if err != nil {
					return nil, err
				}
				if entryChanged {
					changed = true
				}
			}
		}

		tagged, err := containsTaggedFields(sc.Schema(), entryPath(key), entry)
		if err != nil {
			return nil, err
		}
		switch {
		case ref == "" && usesKeyring(snapshot) && tagged:
			dirty = append(dirty, entryPath(key))
		case ref != "" && !usesKeyring(snapshot):
			dirty = append(dirty, entryPath(key))
		}
	}

	return &coordinated.StabilizeResult{
		Data:            data,
		Changed:         changed,
		ProjectionDirty: dirty,
	}, nil
}

func (c *credentialsCoordinator) Save(ctx context.Context, sc coordinated.SaveContext, changes document.JSONPatchSet) error {
	beforeLogical := sc.Logical()
	afterLogical, err := sc.LogicalAfter(changes)
	if err != nil {
		return err
	}

	for _, key := range changedKeys(beforeLogical, afterLogical, changes) {
		beforeRaw, _, err := entryValueAt(beforeLogical, entryPath(key))
		if err != nil {
			return err
		}
		afterRaw, afterExists, err := entryValueAt(afterLogical, entryPath(key))
		if err != nil {
			return err
		}

		existingRef, _ := stringAt(beforeRaw, refPath)
		if !afterExists {
			if existingRef != "" {
				c.secrets.Delete(existingRef)
			}
			if err := c.writeMetadata(ctx, entryPath(key), nil); err != nil {
				return err
			}
			continue
		}

		if usesKeyring(afterLogical) {
			metadata, secret, err := splitTaggedFields(sc.Schema(), entryPath(key), afterRaw)
			if err != nil {
				return err
			}

			ref := existingRef
			if ref == "" {
				ref = "profile/" + key
			}
			c.secrets.Set(ref, secret)

			if result := jsonptr.SetPath(metadata, refPath, ref); !result.Success {
				return fmt.Errorf("failed to set secret ref at %q", refPath)
			}
			if err := c.writeMetadata(ctx, entryPath(key), metadata); err != nil {
				return err
			}
			continue
		}

		if existingRef != "" {
			c.secrets.Delete(existingRef)
		}
		metadata := container.DeepCopyMap(afterRaw)
		jsonptr.DeletePath(metadata, refPath)
		if err := c.writeMetadata(ctx, entryPath(key), metadata); err != nil {
			return err
		}
	}

	return nil
}

func (c *credentialsCoordinator) baseData(snapshot map[string]any) map[string]any {
	base := c.loaded
	if base == nil {
		base = make(map[string]any)
	}
	data := container.DeepCopyMap(base)
	if currentRoot, ok := jsonptr.GetPath(snapshot, pathCredential); ok {
		jsonptr.SetPath(data, pathCredential, container.DeepCopyValue(currentRoot))
	}
	return data
}

func (c *credentialsCoordinator) writeMetadata(ctx context.Context, path string, metadata map[string]any) error {
	current, err := c.metadata.Load(ctx)
	if err != nil {
		return err
	}

	var patches document.JSONPatchSet
	if metadata == nil {
		if jsonptr.DeletePath(current, path) {
			patches.Remove(path)
		}
	} else {
		_, existed := jsonptr.GetPath(current, path)
		if result := jsonptr.SetPath(current, path, metadata); !result.Success {
			return fmt.Errorf("failed to set metadata for %q", path)
		}
		if existed {
			patches.Replace(path, metadata)
		} else {
			patches.Add(path, metadata)
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

func usesKeyring(logical map[string]any) bool {
	value, _ := jsonptr.GetPath(logical, "/auth/credential_backend")
	backend, _ := value.(string)
	return backend == "keyring"
}

func entryPath(key string) string {
	return pathCredential + "/" + jsonptr.Escape(key)
}

func changedKeys(before, after map[string]any, changes document.JSONPatchSet) []string {
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
		if !coordinated.HasPathPrefix(patch.Path, pathCredential) {
			continue
		}
		segments, err := jsonptr.Parse(patch.Path)
		if err != nil {
			continue
		}
		if len(segments) <= 1 {
			addAll(mapEntriesAt(before, pathCredential))
			addAll(mapEntriesAt(after, pathCredential))
			continue
		}
		addKey(segments[1])
	}

	sort.Strings(keys)
	return keys
}

func hydrateTaggedFields(schema coordinated.SchemaView, root string, entry map[string]any, payload map[string]any) (bool, error) {
	fields, err := coordinated.TaggedDescendants(schema, root, externalTagKey, externalTagValue)
	if err != nil {
		return false, err
	}

	changed := false
	for _, field := range fields {
		rel := strings.TrimPrefix(field.Path(), root)
		value, ok := jsonptr.GetPath(payload, rel)
		if !ok {
			continue
		}
		current, exists := jsonptr.GetPath(entry, rel)
		if exists && reflect.DeepEqual(current, value) {
			continue
		}
		if result := jsonptr.SetPath(entry, rel, value); !result.Success {
			return false, fmt.Errorf("failed to hydrate %q", rel)
		}
		changed = true
	}

	return changed, nil
}

func splitTaggedFields(schema coordinated.SchemaView, root string, entry map[string]any) (map[string]any, map[string]any, error) {
	metadata := container.DeepCopyMap(entry)
	secret := make(map[string]any)

	fields, err := coordinated.TaggedDescendants(schema, root, externalTagKey, externalTagValue)
	if err != nil {
		return nil, nil, err
	}
	for _, field := range fields {
		rel := strings.TrimPrefix(field.Path(), root)
		value, ok := jsonptr.GetPath(metadata, rel)
		if !ok {
			continue
		}
		if result := jsonptr.SetPath(secret, rel, value); !result.Success {
			return nil, nil, fmt.Errorf("failed to set secret %q", rel)
		}
		if deleted := jsonptr.DeletePath(metadata, rel); !deleted {
			return nil, nil, fmt.Errorf("failed to delete metadata %q", rel)
		}
	}

	return metadata, secret, nil
}

func containsTaggedFields(schema coordinated.SchemaView, root string, entry map[string]any) (bool, error) {
	fields, err := coordinated.TaggedDescendants(schema, root, externalTagKey, externalTagValue)
	if err != nil {
		return false, err
	}
	for _, field := range fields {
		rel := strings.TrimPrefix(field.Path(), root)
		if _, ok := jsonptr.GetPath(entry, rel); ok {
			return true, nil
		}
	}
	return false, nil
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
		return nil, false, fmt.Errorf("value at %q is %T, want map[string]any", path, value)
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
