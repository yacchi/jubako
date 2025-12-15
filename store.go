package jubako

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/yacchi/jubako/container"
	"github.com/yacchi/jubako/decoder"
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
)

// subscriber wraps a callback function with a unique ID for reliable unsubscription.
type subscriber[T any] struct {
	id uint64
	fn func(T)
}

// LayerInfo provides metadata about a registered layer.
// This interface is implemented by internal layer entries to avoid
// allocating new structs during traversal operations.
type LayerInfo interface {
	// Name returns the unique identifier for this layer.
	Name() layer.Name

	// Priority returns the layer's merge priority.
	Priority() layer.Priority

	// Format returns the document format (e.g., "yaml", "toml", "env").
	// Returns empty string if the layer has not been loaded yet.
	Format() document.DocumentFormat

	// Path returns the file path for file-based layers.
	// Returns empty string for non-file layers (e.g., embedded strings, environment variables).
	Path() string

	// Loaded returns whether the layer has been loaded.
	Loaded() bool

	// ReadOnly returns whether the layer is marked as read-only.
	// A read-only layer cannot be modified via SetTo, even if the underlying
	// source supports saving.
	ReadOnly() bool

	// Writable returns whether the layer supports saving.
	// Returns false if the layer is marked as read-only or if the underlying
	// layer doesn't implement WritableLayer.
	Writable() bool

	// Dirty returns whether the layer has been modified but not yet saved.
	Dirty() bool
}

// AddOption is a functional option for configuring layer addition.
type AddOption func(*addOptions)

// addOptions holds the options for Add method.
type addOptions struct {
	priority    layer.Priority
	hasPriority bool
	readOnly    bool
}

// WithPriority sets a specific priority for the layer.
// Higher priority values override lower priority values during merging.
func WithPriority(p layer.Priority) AddOption {
	return func(o *addOptions) {
		o.priority = p
		o.hasPriority = true
	}
}

// WithReadOnly marks the layer as read-only, preventing modifications via SetTo.
// This is useful for system-wide configuration files that should not be modified,
// even if the underlying source supports saving.
func WithReadOnly() AddOption {
	return func(o *addOptions) {
		o.readOnly = true
	}
}

// layerEntry holds a layer with its priority for internal use.
// It implements LayerInfo interface to avoid allocating new structs during traversal.
type layerEntry struct {
	layer    layer.Layer
	priority layer.Priority

	// Pre-computed values for efficiency during traversal
	path string // cached file path (empty for non-file sources)

	// readOnly indicates whether the layer is marked as read-only by the user
	readOnly bool

	// dirty indicates whether the layer has been modified but not yet saved
	dirty bool

	// data holds the cached data from Load()
	data map[string]any

	// changeset holds modifications since last Load/Save (for comment preservation)
	changeset document.JSONPatchSet
}

// Name returns the unique identifier for this layer.
func (e *layerEntry) Name() layer.Name {
	return e.layer.Name()
}

// Priority returns the layer's merge priority.
func (e *layerEntry) Priority() layer.Priority {
	return e.priority
}

// Format returns the document format.
// Returns empty string if the layer doesn't implement FormatProvider.
func (e *layerEntry) Format() document.DocumentFormat {
	if fp, ok := e.layer.(layer.FormatProvider); ok {
		return fp.Format()
	}
	return ""
}

// Path returns the file path for file-based layers.
// Returns empty string for non-file layers.
func (e *layerEntry) Path() string {
	return e.path
}

// Loaded returns whether the layer has been loaded.
func (e *layerEntry) Loaded() bool {
	return e.data != nil
}

// ReadOnly returns whether the layer is marked as read-only.
func (e *layerEntry) ReadOnly() bool {
	return e.readOnly
}

// Writable returns whether the layer supports saving.
// Returns false if the layer is marked as read-only or if the underlying
// layer doesn't support saving.
func (e *layerEntry) Writable() bool {
	if e.readOnly {
		return false
	}
	return e.layer.CanSave()
}

// Dirty returns whether the layer has been modified but not yet saved.
func (e *layerEntry) Dirty() bool {
	return e.dirty
}

// findLayerLocked finds a layer by name.
// Returns nil if the layer is not found.
// Caller must hold the lock (read or write).
func (s *Store[T]) findLayerLocked(name layer.Name) *layerEntry {
	for _, entry := range s.layers {
		if entry.layer.Name() == name {
			return entry
		}
	}
	return nil
}

// StoreOption is a functional option for configuring Store creation.
type StoreOption func(*storeOptions)

// storeOptions holds the options for New.
type storeOptions struct {
	priorityStep int
	decoder      MapDecoder
}

// defaultPriorityStep is the default step size for auto-assigned priorities.
const defaultPriorityStep = 10

// WithPriorityStep sets the step size for auto-assigned priorities.
// When layers are added without explicit priority, they are assigned
// priorities in increments of this step (0, step, 2*step, ...).
// This allows inserting layers between existing ones with explicit priorities.
// Default is 10.
func WithPriorityStep(step int) StoreOption {
	return func(o *storeOptions) {
		o.priorityStep = step
	}
}

// WithDecoder sets a custom map decoder for the Store.
// The decoder is used to convert the merged map[string]any into the target struct.
// The default decoder uses JSON marshal/unmarshal.
//
// This allows using alternative decoders like mapstructure for more flexible decoding.
//
// Example:
//
//	// Using mapstructure (hypothetical)
//	store := jubako.New[AppConfig](jubako.WithDecoder(mapstructureDecoder))
func WithDecoder(decoder MapDecoder) StoreOption {
	return func(o *storeOptions) {
		o.decoder = decoder
	}
}

// Store manages multiple configuration layers and provides a materialized view
// of the merged configuration.
//
// Get() returns a snapshot of the current resolved configuration.
// Use Subscribe() to react to updates across Load/Reload.
type Store[T any] struct {
	// layers holds all registered configuration layers
	layers []*layerEntry

	// resolved holds the current materialized configuration
	resolved *Cell[T]

	// origins tracks which layer each path's value came from
	origins *origins

	// subscribers holds callbacks for configuration changes
	subscribers []subscriber[T]

	// nextSubID is the next subscriber ID to assign
	nextSubID uint64

	// priorityStep is the step size for auto-assigned priorities
	priorityStep int

	// decoder is the function used to decode map[string]any into T
	// If nil, the default JSON-based decoder is used
	decoder MapDecoder

	// mappingTable holds pre-computed path mappings from jubako struct tags
	// Built once at initialization, used during every materialize
	mappingTable *MappingTable

	// mu protects layers, origins, and subscribers
	mu sync.RWMutex
}

// New creates a new Store for the given configuration type.
// The Store is initialized with a zero value of type T.
//
// Options:
//   - WithPriorityStep(step): Set the step size for auto-assigned priorities (default: 10)
//   - WithDecoder(decoder): Set a custom map decoder (default: JSON marshal/unmarshal)
//
// Example:
//
//	store := jubako.New[AppConfig]()
//	storeWithStep := jubako.New[AppConfig](jubako.WithPriorityStep(100))
func New[T any](opts ...StoreOption) *Store[T] {
	// Apply options
	options := storeOptions{
		priorityStep: defaultPriorityStep,
		// Set default decoder if not provided
		decoder: decoder.JSON,
	}
	for _, opt := range opts {
		opt(&options)
	}

	var zero T

	// Build mapping table from T's struct tags at initialization time
	table := buildMappingTable(reflect.TypeOf(zero))

	return &Store[T]{
		layers:       make([]*layerEntry, 0),
		resolved:     NewCell(zero),
		origins:      newOrigins(),
		subscribers:  make([]subscriber[T], 0),
		nextSubID:    1,
		priorityStep: options.priorityStep,
		decoder:      options.decoder,
		mappingTable: table,
	}
}

// Add registers a new configuration layer.
// If WithPriority option is provided, layers are sorted by priority (higher overrides lower).
// If no priority is specified, layers are processed in the order they were added.
// If a layer with the same name already exists, it returns an error.
//
// Note: Add does not automatically load the layer. Call Load() or Reload() to load all layers.
//
// Example:
//
//	// With explicit priority
//	store.Add(layer.New("defaults", bytes.FromString(defaultsYAML), yaml.New()), WithPriority(jubako.PriorityDefaults))
//	store.Add(layer.New("user", fs.New("~/.config/app.yaml"), yaml.New()), WithPriority(jubako.PriorityUser))
//
//	// Without priority (processed in addition order)
//	store.Add(layer.New("base", bytes.FromString(baseYAML), yaml.New()))
//	store.Add(layer.New("override", bytes.FromString(overrideYAML), yaml.New()))
func (s *Store[T]) Add(l layer.Layer, opts ...AddOption) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate layer names
	for _, entry := range s.layers {
		if entry.layer.Name() == l.Name() {
			return fmt.Errorf("layer %q already exists", l.Name())
		}
	}

	// Apply options
	var options addOptions
	for _, opt := range opts {
		opt(&options)
	}

	// Determine priority: use explicit value or auto-assign based on insertion order
	priority := options.priority
	if !options.hasPriority {
		// Auto-assign priority based on current count, with gaps for future insertions
		priority = layer.Priority(len(s.layers) * s.priorityStep)
	}

	entry := &layerEntry{
		layer:    l,
		priority: priority,
		readOnly: options.readOnly,
	}

	// Try to get file path from FileLayer's source
	if fl, ok := l.(*layer.FileLayer); ok {
		if pp, ok := fl.Source().(PathProvider); ok {
			entry.path = pp.Path()
		}
	}

	s.layers = append(s.layers, entry)

	// Keep layers sorted by priority (stable sort preserves insertion order for same priority)
	sort.SliceStable(s.layers, func(i, j int) bool {
		return s.layers[i].priority < s.layers[j].priority
	})

	return nil
}

// Get returns the current materialized configuration.
// The returned value is a snapshot at the time of the call.
// For reactive updates, use Subscribe() instead.
func (s *Store[T]) Get() T {
	return s.resolved.Get()
}

// Subscribe registers a callback that will be called whenever the configuration changes.
// Returns an unsubscribe function that removes the callback when called.
// The unsubscribe function is safe to call multiple times.
//
// Example:
//
//	unsubscribe := store.Subscribe(func(cfg AppConfig) {
//	  log.Printf("Config changed: %+v", cfg)
//	})
//	defer unsubscribe()
func (s *Store[T]) Subscribe(fn func(T)) func() {
	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	s.subscribers = append(s.subscribers, subscriber[T]{id: id, fn: fn})
	s.mu.Unlock()

	// Return unsubscribe function
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, sub := range s.subscribers {
			if sub.id == id {
				s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
				return
			}
		}
	}
}

// notifySubscribers calls all registered subscribers with the current configuration.
func (s *Store[T]) notifySubscribers() {
	s.mu.RLock()
	current := s.resolved.Get()
	subscribers := append([]subscriber[T](nil), s.subscribers...)
	s.mu.RUnlock()

	for _, sub := range subscribers {
		sub.fn(current)
	}
}

// Load loads all registered layers and materializes the configuration.
// This should be called after all layers have been registered with Add.
//
// Example:
//
//	store := jubako.New[AppConfig]()
//	store.Add(layer.New("defaults", bytes.FromString(defaultsYAML), yaml.New()), WithPriority(jubako.PriorityDefaults))
//	store.Add(layer.New("user", fs.New("~/.config/app.yaml"), yaml.New()), WithPriority(jubako.PriorityUser))
//	if err := store.Load(context.Background()); err != nil {
//	  log.Fatal(err)
//	}
func (s *Store[T]) Load(ctx context.Context) error {
	s.mu.Lock()

	// Load each layer's data
	for _, entry := range s.layers {
		// Load data through the layer interface
		data, err := entry.layer.Load(ctx)
		if err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to load layer %q: %w", entry.layer.Name(), err)
		}
		// Store the loaded data in the entry
		entry.data = data
		// Clear changeset as we have fresh data
		entry.changeset = nil
		// Clear dirty flag
		entry.dirty = false
	}

	// Materialize the merged configuration
	current, subscribers, err := s.materializeLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	for _, sub := range subscribers {
		sub.fn(current)
	}
	return nil
}

// Reload reloads all layers and re-materializes the configuration.
// Unlike Load(), Reload preserves any uncommitted changesets and reapplies them
// after loading fresh data from sources. This enables optimistic locking patterns
// where local changes are preserved even when the underlying source changes.
//
// Example:
//
//	if err := store.Reload(context.Background()); err != nil {
//	  log.Printf("Failed to reload config: %v", err)
//	}
func (s *Store[T]) Reload(ctx context.Context) error {
	s.mu.Lock()

	// Save existing changesets before reloading
	savedChangesets := make(map[layer.Name][]document.JSONPatch)
	for _, entry := range s.layers {
		if len(entry.changeset) > 0 {
			savedChangesets[entry.layer.Name()] = entry.changeset
		}
	}

	// Load each layer's data
	for _, entry := range s.layers {
		data, err := entry.layer.Load(ctx)
		if err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to load layer %q: %w", entry.layer.Name(), err)
		}
		// Store the loaded data in the entry
		entry.data = data
	}

	// Reapply saved changesets
	for _, entry := range s.layers {
		changeset, exists := savedChangesets[entry.layer.Name()]
		if !exists {
			entry.changeset = nil
			entry.dirty = false
			continue
		}

		// Reapply each patch operation
		for _, patch := range changeset {
			switch patch.Op {
			case document.PatchOpAdd, document.PatchOpReplace:
				jsonptr.SetPath(entry.data, patch.Path, patch.Value)
			case document.PatchOpRemove:
				jsonptr.DeletePath(entry.data, patch.Path)
			}
		}

		// Restore the changeset and dirty flag
		entry.changeset = changeset
		entry.dirty = true
	}

	// Materialize the merged configuration
	current, subscribers, err := s.materializeLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	for _, sub := range subscribers {
		sub.fn(current)
	}
	return nil
}

// SetTo sets a value in a specific layer at the given JSONPointer path.
// The layer's data is updated in memory, but not persisted until Save() is called.
//
// Example:
//
//	// Set server port in user layer
//	err := store.SetTo("user", "/server/port", 9000)
//	if err != nil {
//	  log.Fatal(err)
//	}
//
//	// Save the change to disk
//	err = store.SaveLayer(ctx, "user")
func (s *Store[T]) SetTo(layerName layer.Name, path string, value any) error {
	s.mu.Lock()

	entry := s.findLayerLocked(layerName)
	if entry == nil {
		s.mu.Unlock()
		return fmt.Errorf("layer %q not found", layerName)
	}

	if !entry.Writable() {
		if entry.readOnly {
			s.mu.Unlock()
			return fmt.Errorf("layer %q is marked as read-only", layerName)
		}
		s.mu.Unlock()
		return fmt.Errorf("layer %q does not support saving (source is not writable)", layerName)
	}

	if entry.data == nil {
		s.mu.Unlock()
		return fmt.Errorf("layer %q has not been loaded", layerName)
	}

	// Set the value in the data map using jsonptr.SetPath
	// SetResult tells us whether this was a create (add) or update (replace) operation
	result := jsonptr.SetPath(entry.data, path, value)
	if !result.Success {
		s.mu.Unlock()
		return fmt.Errorf("failed to set value at path %q", path)
	}

	// Determine the patch operation based on SetResult
	var op document.PatchOp
	if result.Created {
		op = document.PatchOpAdd
	} else {
		op = document.PatchOpReplace
	}

	// Record the change to changeset for comment preservation
	entry.changeset = append(entry.changeset, document.JSONPatch{
		Op:    op,
		Path:  path,
		Value: value,
	})

	// Mark the layer as dirty
	entry.dirty = true

	// Re-materialize to update the resolved config
	current, subscribers, err := s.materializeLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	for _, sub := range subscribers {
		sub.fn(current)
	}
	return nil
}

// Save persists all modified (dirty) layers to their sources.
// Only layers that have been modified via SetTo() and support saving will be persisted.
// After successful save, the dirty flag is cleared for each saved layer.
//
// Example:
//
//	// Modify layers
//	store.SetTo("user", "/server/port", 9000)
//	store.SetTo("project", "/database/host", "localhost")
//
//	// Save all modified layers
//	if err := store.Save(context.Background()); err != nil {
//	  log.Fatal(err)
//	}
func (s *Store[T]) Save(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error

	for _, entry := range s.layers {
		// Skip layers that haven't been modified
		// Note: dirty flag is only set by SetTo, which requires writable layer
		if !entry.dirty {
			continue
		}

		if err := s.saveLayerLocked(ctx, entry); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// SaveLayer persists a specific layer's document to its source.
// Returns an error if the layer doesn't support saving (doesn't implement WritableLayer).
// After successful save, the dirty flag is cleared for the layer.
//
// Example:
//
//	// Modify user layer
//	store.SetTo("user", "/server/port", 9000)
//
//	// Save only user layer to file
//	if err := store.SaveLayer(context.Background(), "user"); err != nil {
//	  log.Fatal(err)
//	}
func (s *Store[T]) SaveLayer(ctx context.Context, layerName layer.Name) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.findLayerLocked(layerName)
	if entry == nil {
		return fmt.Errorf("layer %q not found", layerName)
	}

	return s.saveLayerLocked(ctx, entry)
}

// saveLayerLocked saves a single layer entry.
// Caller must hold the lock.
func (s *Store[T]) saveLayerLocked(ctx context.Context, entry *layerEntry) error {
	if !entry.layer.CanSave() {
		return fmt.Errorf("layer %q does not support saving", entry.layer.Name())
	}

	if entry.data == nil {
		return fmt.Errorf("layer %q has not been loaded", entry.layer.Name())
	}

	if err := entry.layer.Save(ctx, entry.changeset); err != nil {
		return fmt.Errorf("failed to save layer %q: %w", entry.layer.Name(), err)
	}

	// Clear dirty flag and changeset on successful save
	entry.dirty = false
	entry.changeset = nil

	return nil
}

// GetLayer returns the layer with the given name, or nil if not found.
// This is useful for accessing layer-specific information.
func (s *Store[T]) GetLayer(name layer.Name) layer.Layer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if entry := s.findLayerLocked(name); entry != nil {
		return entry.layer
	}
	return nil
}

// GetAt returns the resolved value at the given JSON Pointer path along with
// its origin information. The ResolvedValue includes the value, whether it was set,
// and which layer it came from.
//
// For leaf values (scalars), the value and origin are returned directly.
// For container values (maps/slices), the merged value from all layers is returned,
// and the origin is the highest priority layer that has a value at that path.
//
// Example:
//
//	rv := store.GetAt("/server/port")
//	if rv.Exists {
//	  fmt.Printf("port=%v (from layer %s)\n", rv.Value, rv.Layer.Name())
//	}
func (s *Store[T]) GetAt(path string) ResolvedValue {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getAtLocked(path)
}

// getAtLocked returns the resolved value at path. Caller must hold the lock.
func (s *Store[T]) getAtLocked(path string) ResolvedValue {
	// First, check if it's a known leaf path
	if entry := s.origins.getLeaf(path); entry != nil {
		return newResolvedValue(entry, path)
	}

	// Check if it's a known container path
	if entry := s.origins.getContainer(path); entry != nil {
		// Container path - need to compute merged value
		return s.resolveContainerLocked(path, entry)
	}

	// Path not found in either leafs or containers
	return ResolvedValue{}
}

// resolveContainerLocked computes the merged value for a container path.
// The origin is the highest priority layer that has a value at that path.
// Caller must hold the lock.
func (s *Store[T]) resolveContainerLocked(path string, topEntry *layerEntry) ResolvedValue {
	// Get all entries for this container path
	entries := s.origins.getAllContainer(path)
	if len(entries) == 0 {
		return ResolvedValue{}
	}

	// Merge values from all layers (lowest priority first)
	var merged any
	for _, entry := range entries {
		if entry.data == nil {
			continue
		}

		value, ok := jsonptr.GetPath(entry.data, path)
		if !ok {
			continue
		}

		if merged == nil {
			merged = container.DeepCopyValue(value)
		} else {
			// Merge this layer's value into merged
			merged = mergeValues(merged, value)
		}
	}

	if merged == nil {
		return ResolvedValue{}
	}

	return ResolvedValue{
		Value:  merged,
		Exists: true,
		Layer:  topEntry,
	}
}

// GetAllAt returns all values at the given JSON Pointer path from all layers
// that have a value at that path. The results are sorted by priority (lowest first).
// Use Effective() to get the highest priority value.
//
// For container paths (maps/slices), each layer's raw value is returned (not merged).
// This allows callers to see what each layer contributes.
//
// Example:
//
//	values := store.GetAllAt("/server/port")
//	for _, rv := range values {
//	  fmt.Printf("port=%v (from layer %s, priority %d)\n",
//	    rv.Value, rv.Layer.Name(), rv.Layer.Priority())
//	}
//	effective := values.Effective()
//	fmt.Printf("effective: %v\n", effective.Value)
func (s *Store[T]) GetAllAt(path string) ResolvedValues {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check leaf paths first
	if entries := s.origins.getAllLeaf(path); len(entries) > 0 {
		results := make(ResolvedValues, 0, len(entries))
		for _, entry := range entries {
			if rv := newResolvedValue(entry, path); rv.Exists {
				results = append(results, rv)
			}
		}
		return results
	}

	// Check container paths
	if entries := s.origins.getAllContainer(path); len(entries) > 0 {
		results := make(ResolvedValues, 0, len(entries))
		for _, entry := range entries {
			if rv := newResolvedValue(entry, path); rv.Exists {
				results = append(results, rv)
			}
		}
		return results
	}

	return nil
}

// Walk traverses all paths in the resolved configuration, calling fn for each path.
// The paths are visited in alphabetical order.
// The WalkContext provides access to the path and allows lazy retrieval of values.
// If fn returns false, the walk stops.
//
// Example - get resolved value for each path:
//
//	store.Walk(func(ctx WalkContext) bool {
//	  rv := ctx.Value()
//	  fmt.Printf("%s = %v (from %s)\n", ctx.Path, rv.Value, rv.Layer.Name())
//	  return true // continue walking
//	})
//
// Example - get all values from all layers:
//
//	store.Walk(func(ctx WalkContext) bool {
//	  for _, rv := range ctx.AllValues() {
//	    fmt.Printf("%s = %v (layer %s, priority %d)\n",
//	      ctx.Path, rv.Value, rv.Layer.Name(), rv.Layer.Priority())
//	  }
//	  return true
//	})
//
// Example - collect only paths:
//
//	var paths []string
//	store.Walk(func(ctx WalkContext) bool {
//	  paths = append(paths, ctx.Path)
//	  return true
//	})
func (s *Store[T]) Walk(fn func(ctx WalkContext) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Walk only visits leaf paths (not containers)
	paths := make([]string, 0, len(s.origins.leafs))
	for path := range s.origins.leafs {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		orig := s.origins.leafs[path]
		ctx := WalkContext{
			Path:   path,
			origin: orig,
		}

		if !fn(ctx) {
			return
		}
	}
}

// PathProvider is an optional interface that sources can implement
// to provide their file path.
type PathProvider interface {
	Path() string
}

// GetLayerInfo returns metadata about a specific layer.
// Returns nil if the layer is not found.
//
// Example:
//
//	info := store.GetLayerInfo("user")
//	if info != nil {
//	  fmt.Printf("Layer: %s, Priority: %d, Path: %s\n", info.Name(), info.Priority(), info.Path())
//	}
func (s *Store[T]) GetLayerInfo(name layer.Name) LayerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if entry := s.findLayerLocked(name); entry != nil {
		return entry
	}
	return nil
}

// ListLayers returns metadata about all registered layers, sorted by priority.
//
// Example:
//
//	layers := store.ListLayers()
//	for _, info := range layers {
//	  fmt.Printf("  %s (priority: %d)\n", info.Name(), info.Priority())
//	}
func (s *Store[T]) ListLayers() []LayerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]LayerInfo, len(s.layers))
	for i, entry := range s.layers {
		result[i] = entry
	}
	return result
}

// IsDirty returns true if any layer has been modified but not yet saved.
//
// Example:
//
//	if store.IsDirty() {
//	  // Prompt user to save changes
//	  if err := store.Save(ctx); err != nil {
//	    log.Fatal(err)
//	  }
//	}
func (s *Store[T]) IsDirty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, entry := range s.layers {
		if entry.dirty {
			return true
		}
	}
	return false
}

// MappingTable returns the path mapping table derived from jubako struct tags.
// Returns nil if the type T has no jubako tag mappings.
// The returned MappingTable can be inspected programmatically or printed via String().
//
// Example:
//
//	store := jubako.New[ServerConfig]()
//	table := store.MappingTable()
//	if table != nil {
//	    for _, m := range table.Mappings {
//	        fmt.Printf("%s <- %s\n", m.FieldKey, m.SourcePath)
//	    }
//	}
//	// Or simply print:
//	fmt.Println(table)
func (s *Store[T]) MappingTable() *MappingTable {
	return s.mappingTable
}

// HasMappings returns true if the struct type T has any jubako tag mappings defined.
func (s *Store[T]) HasMappings() bool {
	return s.mappingTable != nil && !s.mappingTable.IsEmpty()
}
