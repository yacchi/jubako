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
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/types"
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

	// NoWatch returns whether watching is disabled for this layer.
	NoWatch() bool

	// Sensitive returns whether the layer is marked as containing sensitive data.
	// Sensitive layers can only contain sensitive fields, and sensitive fields
	// can only be written to sensitive layers.
	Sensitive() bool

	// Optional returns whether the layer is marked as optional.
	// Optional layers do not cause an error if their source does not exist.
	Optional() bool
}

// AddOption is a functional option for configuring layer addition.
type AddOption func(*addOptions)

// addOptions holds the options for Add method.
type addOptions struct {
	priority    layer.Priority
	hasPriority bool
	readOnly    bool
	noWatch     bool
	sensitive   bool
	optional    bool
}

// WithPriority sets a specific priority for the layer.
// Higher priority values override lower priority values during merging.
func WithPriority(p layer.Priority) AddOption {
	return func(o *addOptions) {
		o.priority = p
		o.hasPriority = true
	}
}

// WithReadOnly marks the layer as read-only, preventing modifications via SetTo
// and preventing writes via Save/SaveLayer.
// This is useful for system-wide configuration files that should never be modified,
// even if the underlying source supports saving.
func WithReadOnly() AddOption {
	return func(o *addOptions) {
		o.readOnly = true
	}
}

// WithNoWatch disables watching for this layer.
// When Store.Watch() is called, layers marked with WithNoWatch will be skipped.
// This is useful for layers that should not trigger reloads (e.g., static defaults,
// embedded configurations).
func WithNoWatch() AddOption {
	return func(o *addOptions) {
		o.noWatch = true
	}
}

// WithSensitive marks the layer as containing sensitive data.
// Sensitive layers can only accept writes to fields marked with `jubako:"sensitive"` tag.
// Conversely, fields marked as sensitive can only be written to sensitive layers.
// This prevents cross-contamination of sensitive data (e.g., credentials) with
// regular configuration data.
func WithSensitive() AddOption {
	return func(o *addOptions) {
		o.sensitive = true
	}
}

// WithOptional marks the layer as optional.
// When enabled, if the layer's source does not exist (returns source.ErrNotExist),
// the layer is treated as empty instead of causing a load error.
// This is useful for optional user configuration files that may not exist initially.
//
// When Save is called on an optional layer that was initially missing,
// the file will be created at the configured path.
//
// Example:
//
//	// User config is optional - application works with defaults if not present
//	store.Add(
//	    layer.New("user", fs.New("~/.config/app/config.yaml"), yaml.New()),
//	    jubako.WithOptional(),
//	)
func WithOptional() AddOption {
	return func(o *addOptions) {
		o.optional = true
	}
}

// layerEntry holds a layer with its priority for internal use.
// It implements LayerInfo interface to avoid allocating new structs during traversal.
type layerEntry struct {
	layer    layer.Layer
	priority layer.Priority

	// Pre-computed layer details (path, format, etc.)
	details types.Details

	// readOnly indicates whether the layer is marked as read-only by the user
	readOnly bool

	// noWatch indicates whether watching is disabled for this layer
	noWatch bool

	// sensitive indicates whether the layer contains sensitive data
	sensitive bool

	// optional indicates whether the layer source is optional (missing source is not an error)
	optional bool

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
// Returns empty string if not available.
func (e *layerEntry) Format() document.DocumentFormat {
	return e.details.Format
}

// Path returns the file path for file-based layers.
// Returns empty string for non-file layers.
func (e *layerEntry) Path() string {
	return e.details.Path
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

// NoWatch returns whether watching is disabled for this layer.
func (e *layerEntry) NoWatch() bool {
	return e.noWatch
}

// Sensitive returns whether the layer is marked as containing sensitive data.
func (e *layerEntry) Sensitive() bool {
	return e.sensitive
}

// Optional returns whether the layer is marked as optional.
func (e *layerEntry) Optional() bool {
	return e.optional
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
	priorityStep  int
	decoder       MapDecoder
	sensitiveMask SensitiveMaskFunc
	tagDelimiter  string
	tagName       string
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

// WithSensitiveMask sets a custom mask handler for sensitive fields.
// When sensitive fields are accessed through GetAt or Walk, the mask function
// is called to transform the value before returning it.
//
// Use GetAtUnmasked to retrieve the original unmasked value when needed.
//
// Example:
//
//	store := jubako.New[Config](jubako.WithSensitiveMask(func(value any) any {
//	    if s, ok := value.(string); ok {
//	        if len(s) > 4 {
//	            return s[:2] + "****" + s[len(s)-2:]
//	        }
//	    }
//	    return "****"
//	}))
func WithSensitiveMask(fn SensitiveMaskFunc) StoreOption {
	return func(o *storeOptions) {
		o.sensitiveMask = fn
	}
}

// WithSensitiveMaskString sets a fixed mask string for sensitive fields.
// This is a convenience wrapper around WithSensitiveMask that returns a constant string.
//
// Example:
//
//	store := jubako.New[Config](jubako.WithSensitiveMaskString("[REDACTED]"))
func WithSensitiveMaskString(mask string) StoreOption {
	return func(o *storeOptions) {
		o.sensitiveMask = func(any) any {
			return mask
		}
	}
}

// WithTagDelimiter sets a custom delimiter for separating path and directives
// in jubako struct tags. The default delimiter is "," (comma).
//
// Use this option when your configuration keys contain commas and you need
// to map to paths containing those commas.
//
// Example:
//
//	// Default: `jubako:"/path,sensitive"` means path="/path" + directive="sensitive"
//	store := jubako.New[Config]()
//
//	// With semicolon delimiter: `jubako:"/path,with,commas;sensitive"`
//	// means path="/path,with,commas" + directive="sensitive"
//	store := jubako.New[Config](jubako.WithTagDelimiter(";"))
func WithTagDelimiter(delimiter string) StoreOption {
	return func(o *storeOptions) {
		o.tagDelimiter = delimiter
	}
}

// WithTagName sets the struct tag name used for field name resolution.
// The default is "json", following the same convention as encoding/json.
//
// This is useful when using custom decoders that expect different tag names
// (e.g., "yaml", "toml", "mapstructure").
//
// The tag value is parsed the same way as encoding/json: split by comma,
// and the first segment is used as the field name.
//
// Example:
//
//	// Default: uses json tag
//	store := jubako.New[Config]()
//	// Field `Server ServerConfig `json:"server"`` is accessed via "/server"
//
//	// With yaml tag
//	store := jubako.New[Config](jubako.WithTagName("yaml"))
//	// Field `Server ServerConfig `yaml:"server"`` is accessed via "/server"
func WithTagName(name string) StoreOption {
	return func(o *storeOptions) {
		o.tagName = name
	}
}

// Store manages multiple configuration layers and provides a materialized view
// of the merged configuration.
//
// Get() returns a snapshot of the current resolved configuration.
// Use Subscribe() to react to updates across Load/Reload.
//
// Store implements layer.StoreProvider, allowing layers to access store-level
// configuration during initialization.
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

	// schema holds the pre-computed mapping structure from jubako struct tags.
	// Built once at initialization, used during every materialize and SetTo validation.
	// Contains both the hierarchical MappingTable (for transformation) and
	// the flat MappingTrie (for path-based lookups).
	schema *Schema

	// sensitiveMask is the function used to mask sensitive values in GetAt and Walk.
	// If nil, sensitive values are returned as-is.
	sensitiveMask SensitiveMaskFunc

	// tagDelimiter is the delimiter used in jubako struct tags
	tagDelimiter string

	// tagName is the struct tag name used for field name resolution
	tagName string

	// mu protects layers, origins, and subscribers
	mu sync.RWMutex
}

// New creates a new Store for the given configuration type.
// The Store is initialized with a zero value of type T.
//
// Options:
//   - WithPriorityStep(step): Set the step size for auto-assigned priorities (default: 10)
//   - WithDecoder(decoder): Set a custom map decoder (default: JSON marshal/unmarshal)
//   - WithTagDelimiter(delimiter): Set a custom delimiter for jubako struct tags (default: ",")
//   - WithTagName(name): Set the struct tag name for field resolution (default: "json")
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
		decoder:      decoder.JSON,
		tagDelimiter: DefaultTagDelimiter,
		tagName:      DefaultFieldTagName,
	}
	for _, opt := range opts {
		opt(&options)
	}

	var zero T

	// Build mapping table from T's struct tags at initialization time
	table := buildMappingTable(reflect.TypeOf(zero), options.tagDelimiter, options.tagName)

	// Build schema containing both table and trie
	schema := NewSchema(table)

	return &Store[T]{
		layers:        make([]*layerEntry, 0),
		resolved:      NewCell(zero),
		origins:       newOrigins(),
		subscribers:   make([]subscriber[T], 0),
		nextSubID:     1,
		priorityStep:  options.priorityStep,
		decoder:       options.decoder,
		schema:        schema,
		sensitiveMask: options.sensitiveMask,
		tagDelimiter:  options.tagDelimiter,
		tagName:       options.tagName,
	}
}

// Ensure Store implements layer.StoreProvider interface.
var _ layer.StoreProvider = (*Store[struct{}])(nil)

// SchemaType returns the reflect.Type of the Store's configuration struct T.
// This implements layer.StoreProvider.
func (s *Store[T]) SchemaType() reflect.Type {
	var zero T
	return reflect.TypeOf(zero)
}

// TagDelimiter returns the delimiter used in jubako struct tags.
// This implements layer.StoreProvider.
func (s *Store[T]) TagDelimiter() string {
	return s.tagDelimiter
}

// FieldTagName returns the struct tag name used for field name resolution.
// This implements layer.StoreProvider.
func (s *Store[T]) FieldTagName() string {
	return s.tagName
}

// Add registers a new configuration layer.
// If WithPriority option is provided, layers are sorted by priority (higher overrides lower).
// If no priority is specified, layers are processed in the order they were added.
// If a layer with the same name already exists, it returns an error.
//
// If the layer was created with layer.Init(), it will be initialized with this Store
// as the StoreProvider, allowing layers to access store-level configuration
// (such as SchemaType for env layers).
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
//
//	// Env layer with auto schema (uses Store's type T for schema mapping)
//	store.Add(env.NewWithAutoSchema("env", "APP_"))
func (s *Store[T]) Add(l layer.Layer, opts ...AddOption) error {
	// If the layer implements StoreAwareLayerInitializer, initialize it with this Store
	if lazy, ok := l.(layer.StoreAwareLayerInitializer); ok {
		l = lazy.InitWithStore(s)
	}

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
		layer:     l,
		priority:  priority,
		readOnly:  options.readOnly,
		noWatch:   options.noWatch,
		sensitive: options.sensitive,
		optional:  options.optional,
	}

	// Populate layer details (Layer interface includes DetailsFiller)
	l.FillDetails(&entry.details)

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
	defer s.mu.Unlock()

	id := s.nextSubID
	s.nextSubID++
	s.subscribers = append(s.subscribers, subscriber[T]{id: id, fn: fn})

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
	current, subscribers, err := s.loadLocked(ctx)
	if err != nil {
		return err
	}
	for _, sub := range subscribers {
		sub.fn(current)
	}
	return nil
}

// loadLocked performs the layer loading and materialization under lock.
// Returns the current configuration and subscribers snapshot for notification outside the lock.
func (s *Store[T]) loadLocked(ctx context.Context) (T, []subscriber[T], error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Load each layer's data
	for _, entry := range s.layers {
		// Load data through the layer interface
		data, err := entry.layer.Load(ctx)
		if err != nil {
			// For optional layers, treat source.ErrNotExist as empty data
			if entry.optional && errors.Is(err, source.ErrNotExist) {
				entry.data = make(map[string]any)
				entry.changeset = nil
				entry.dirty = false
				continue
			}
			var zero T
			return zero, nil, fmt.Errorf("failed to load layer %q: %w", entry.layer.Name(), err)
		}
		// Store the loaded data in the entry
		entry.data = data
		// Clear changeset as we have fresh data
		entry.changeset = nil
		// Clear dirty flag
		entry.dirty = false
	}

	// Materialize the merged configuration
	return s.materializeLocked()
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
	current, subscribers, err := s.reloadLocked(ctx)
	if err != nil {
		return err
	}
	for _, sub := range subscribers {
		sub.fn(current)
	}
	return nil
}

// reloadLocked performs the layer reloading and materialization under lock.
// Unlike loadLocked, it preserves and reapplies uncommitted changesets.
// Returns the current configuration and subscribers snapshot for notification outside the lock.
func (s *Store[T]) reloadLocked(ctx context.Context) (T, []subscriber[T], error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
			// For optional layers, treat source.ErrNotExist as empty data
			if entry.optional && errors.Is(err, source.ErrNotExist) {
				entry.data = make(map[string]any)
				continue
			}
			var zero T
			return zero, nil, fmt.Errorf("failed to load layer %q: %w", entry.layer.Name(), err)
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
	return s.materializeLocked()
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
	current, subscribers, err := s.setToLocked(layerName, path, value)
	if err != nil {
		return err
	}
	for _, sub := range subscribers {
		sub.fn(current)
	}
	return nil
}

// setToLocked performs the value setting and materialization under lock.
// Returns the current configuration and subscribers snapshot for notification outside the lock.
func (s *Store[T]) setToLocked(layerName layer.Name, path string, value any) (T, []subscriber[T], error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.findLayerLocked(layerName)
	if entry == nil {
		var zero T
		return zero, nil, fmt.Errorf("layer %q not found", layerName)
	}

	if !entry.Writable() {
		var zero T
		if entry.readOnly {
			return zero, nil, fmt.Errorf("layer %q is marked as read-only", layerName)
		}
		return zero, nil, fmt.Errorf("layer %q does not support saving (source is not writable)", layerName)
	}

	if entry.data == nil {
		var zero T
		return zero, nil, fmt.Errorf("layer %q has not been loaded", layerName)
	}

	// Validate sensitivity: ensure sensitive fields only go to sensitive layers and vice versa
	if err := validateSensitivity(s.schema.Trie, path, entry.sensitive); err != nil {
		var zero T
		return zero, nil, fmt.Errorf("%w: path %s, layer %s", err, path, layerName)
	}

	// Set the value in the data map using jsonptr.SetPath
	// SetResult tells us whether this was a create (add) or update (replace) operation
	result := jsonptr.SetPath(entry.data, path, value)
	if !result.Success {
		var zero T
		return zero, nil, fmt.Errorf("failed to set value at path %q", path)
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
	return s.materializeLocked()
}

// Set sets multiple values to a layer using functional options.
// This method provides a flexible way to set values with type-safe helpers
// and supports grouping, struct expansion, and behavior modifiers.
//
// Value options:
//   - String(path, value): Set a string value
//   - Int(path, value): Set an integer value
//   - Bool(path, value): Set a boolean value
//   - Float(path, value): Set a float64 value
//   - Value(path, value): Set any value
//   - Struct(path, v): Expand a struct into multiple path-value pairs
//   - Map(path, m): Expand a map into multiple path-value pairs
//   - Path(prefix, opts...): Group options under a common path prefix
//
// Behavior options:
//   - SkipZeroValues(): Skip entries with zero values
//   - DeleteNilValue(): Treat nil values as delete operations
//
// Example:
//
//	// Set multiple values
//	err := store.Set("user",
//	    jubako.Int("/server/port", 8080),
//	    jubako.String("/server/host", "localhost"),
//	)
//
//	// Use Path for grouping
//	err := store.Set("user", jubako.Path("/server",
//	    jubako.Int("port", 8080),
//	    jubako.String("host", "localhost"),
//	))
//
//	// Expand a struct
//	err := store.Set("user",
//	    jubako.Struct("/credential/default", cred),
//	    jubako.SkipZeroValues(),
//	)
func (s *Store[T]) Set(layerName layer.Name, opts ...SetOption) error {
	if len(opts) == 0 {
		return nil
	}

	current, subscribers, err := s.setLocked(layerName, opts)
	if err != nil {
		return err
	}
	for _, sub := range subscribers {
		sub.fn(current)
	}
	return nil
}

// setLocked performs the value setting and materialization under lock.
// Returns the current configuration and subscribers snapshot for notification outside the lock.
func (s *Store[T]) setLocked(layerName layer.Name, opts []SetOption) (T, []subscriber[T], error) {
	// Build configuration from options
	cfg := &setConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// No patches to apply
	if len(cfg.patches) == 0 {
		var zero T
		return zero, nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.findLayerLocked(layerName)
	if entry == nil {
		var zero T
		return zero, nil, fmt.Errorf("layer %q not found", layerName)
	}

	if !entry.Writable() {
		var zero T
		if entry.readOnly {
			return zero, nil, fmt.Errorf("layer %q is marked as read-only", layerName)
		}
		return zero, nil, fmt.Errorf("layer %q does not support saving (source is not writable)", layerName)
	}

	if entry.data == nil {
		var zero T
		return zero, nil, fmt.Errorf("layer %q has not been loaded", layerName)
	}

	// Apply patches
	for _, pv := range cfg.patches {
		// Handle SkipZeroValues
		if cfg.skipZeroValues && isZeroValue(pv.value) {
			continue
		}

		// Handle DeleteNilValue
		if cfg.deleteNilValue && pv.value == nil {
			// Delete operation
			if jsonptr.DeletePath(entry.data, pv.path) {
				entry.changeset = append(entry.changeset, document.JSONPatch{
					Op:   document.PatchOpRemove,
					Path: pv.path,
				})
				entry.dirty = true
			}
			continue
		}

		// Validate sensitivity
		if err := validateSensitivity(s.schema.Trie, pv.path, entry.sensitive); err != nil {
			var zero T
			return zero, nil, fmt.Errorf("%w: path %s, layer %s", err, pv.path, layerName)
		}

		// Set the value
		result := jsonptr.SetPath(entry.data, pv.path, pv.value)
		if !result.Success {
			var zero T
			return zero, nil, fmt.Errorf("failed to set value at path %q", pv.path)
		}

		// Determine the patch operation
		var op document.PatchOp
		if result.Created {
			op = document.PatchOpAdd
		} else {
			op = document.PatchOpReplace
		}

		// Record the change
		entry.changeset = append(entry.changeset, document.JSONPatch{
			Op:    op,
			Path:  pv.path,
			Value: pv.value,
		})

		entry.dirty = true
	}

	// Re-materialize to update the resolved config
	return s.materializeLocked()
}

// DeleteFrom removes values at the specified JSON Pointer paths from a specific layer.
// The layer's data is updated in memory, but not persisted until Save() is called.
// If a path does not exist, it is silently skipped.
//
// Example:
//
//	// Delete a single path
//	err := store.DeleteFrom("user", "/server/deprecated_field")
//	if err != nil {
//	  log.Fatal(err)
//	}
//
//	// Delete multiple paths at once
//	err = store.DeleteFrom("user", "/temp/cache", "/temp/logs", "/deprecated")
//	if err != nil {
//	  log.Fatal(err)
//	}
//
//	// Save the changes to disk
//	err = store.SaveLayer(ctx, "user")
func (s *Store[T]) DeleteFrom(layerName layer.Name, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}

	current, subscribers, err := s.deleteFromLocked(layerName, paths)
	if err != nil {
		return err
	}
	for _, sub := range subscribers {
		sub.fn(current)
	}
	return nil
}

// deleteFromLocked performs the value deletion and materialization under lock.
// Returns the current configuration and subscribers snapshot for notification outside the lock.
func (s *Store[T]) deleteFromLocked(layerName layer.Name, paths []string) (T, []subscriber[T], error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.findLayerLocked(layerName)
	if entry == nil {
		var zero T
		return zero, nil, fmt.Errorf("layer %q not found", layerName)
	}

	if !entry.Writable() {
		var zero T
		if entry.readOnly {
			return zero, nil, fmt.Errorf("layer %q is marked as read-only", layerName)
		}
		return zero, nil, fmt.Errorf("layer %q does not support saving (source is not writable)", layerName)
	}

	if entry.data == nil {
		var zero T
		return zero, nil, fmt.Errorf("layer %q has not been loaded", layerName)
	}

	// Delete each path
	anyDeleted := false
	for _, path := range paths {
		if path == "" {
			continue
		}

		// Attempt to delete the path
		deleted := jsonptr.DeletePath(entry.data, path)
		if deleted {
			// Record the change to changeset for comment preservation
			entry.changeset = append(entry.changeset, document.JSONPatch{
				Op:   document.PatchOpRemove,
				Path: path,
			})
			anyDeleted = true
		}
	}

	// Only mark dirty and re-materialize if something was actually deleted
	if !anyDeleted {
		// Return current state without re-materializing
		current := s.resolved.Get()
		subscribers := append([]subscriber[T](nil), s.subscribers...)
		return current, subscribers, nil
	}

	// Mark the layer as dirty
	entry.dirty = true

	// Re-materialize to update the resolved config
	return s.materializeLocked()
}

// Save persists all modified (dirty) layers to their sources.
// Only layers that have been modified via SetTo() or DeleteFrom() and support saving will be persisted.
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

// SaveLayer persists a specific layer's pending changes to its source.
// If the layer has no pending changes, SaveLayer is a no-op.
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
	if entry.data == nil {
		return fmt.Errorf("layer %q has not been loaded", entry.layer.Name())
	}

	// Avoid rewriting unchanged documents.
	// This is important for comment-preserving formats where "save without changes"
	// could still re-serialize and lose formatting/comments depending on the Document.
	if !entry.dirty || entry.changeset.IsEmpty() {
		return nil
	}

	if entry.readOnly {
		return fmt.Errorf("layer %q is marked as read-only", entry.layer.Name())
	}

	if !entry.layer.CanSave() {
		return fmt.Errorf("layer %q does not support saving", entry.layer.Name())
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
// If the path points to a sensitive field and masking is enabled (via WithSensitiveMask
// or WithSensitiveMaskString), the returned value will be masked and Masked will be true.
// Use GetAtUnmasked to retrieve the original value.
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

	rv := s.getAtLocked(path)
	return s.applyMaskLocked(rv, path)
}

// GetAtUnmasked returns the resolved value at the given JSON Pointer path
// without applying sensitive masking. Use this when you need the actual value
// for processing (not for display or logging).
//
// Example:
//
//	// For display (masked)
//	rv := store.GetAt("/credentials/password")
//	fmt.Printf("Password: %v\n", rv.Value) // Prints: Password: ********
//
//	// For actual use (unmasked)
//	rv = store.GetAtUnmasked("/credentials/password")
//	authenticate(rv.Value.(string)) // Uses actual password
func (s *Store[T]) GetAtUnmasked(path string) ResolvedValue {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getAtLocked(path)
}

// applyMaskLocked applies sensitive masking to a ResolvedValue if needed.
// Caller must hold the lock.
func (s *Store[T]) applyMaskLocked(rv ResolvedValue, path string) ResolvedValue {
	// No masking if no mask function configured or value doesn't exist
	if s.sensitiveMask == nil || !rv.Exists {
		return rv
	}

	// Check if path is sensitive
	if !s.schema.Trie.IsSensitive(path) {
		return rv
	}

	// Don't mask empty values (nil or empty string)
	// This prevents users from thinking a value is set when it's actually empty
	if isEmptyValue(rv.Value) {
		return rv
	}

	// Apply masking
	rv.Value = s.sensitiveMask(rv.Value)
	rv.Masked = true
	return rv
}

// isEmptyValue returns true if the value is considered empty for masking purposes.
// Empty values (nil, empty string) are not masked to avoid misleading users
// into thinking a value is set when it's actually empty.
func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok && s == "" {
		return true
	}
	return false
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
			Path:      path,
			origin:    orig,
			maskFunc:  s.sensitiveMask,
			sensitive: s.schema.Trie.IsSensitive(path),
		}

		if !fn(ctx) {
			return
		}
	}
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

// Schema returns the schema derived from jubako struct tags.
// The Schema contains both the hierarchical MappingTable (for transformation)
// and the flat MappingTrie (for path-based lookups).
//
// Returns nil if the type T has no jubako tag mappings.
//
// Example:
//
//	store := jubako.New[ServerConfig]()
//	schema := store.Schema()
//	if schema != nil {
//	    // Access the table for field iteration
//	    for _, m := range schema.Table.Mappings {
//	        fmt.Printf("%s <- %s\n", m.FieldKey, m.SourcePath)
//	    }
//	    // Use the trie for path-based lookups
//	    if m := schema.Trie.Lookup("/server/password"); m != nil {
//	        fmt.Printf("Found mapping: %s, sensitive: %v\n", m.FieldKey, m.Sensitive)
//	    }
//	}
//	// Or simply print:
//	fmt.Println(schema)
func (s *Store[T]) Schema() *Schema {
	return s.schema
}

// HasMappings returns true if the struct type T has any jubako tag mappings defined.
func (s *Store[T]) HasMappings() bool {
	return s.schema != nil && s.schema.Table != nil && !s.schema.Table.IsEmpty()
}
