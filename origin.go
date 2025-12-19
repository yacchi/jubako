package jubako

import "github.com/yacchi/jubako/jsonptr"

// ResolvedValue represents a configuration value with its origin information.
// It provides the value, whether the key exists, and which layer it came from.
//
// The three states can be distinguished as follows:
//   - Key does not exist: Exists=false, Value=nil, Layer=nil
//   - Explicit null: Exists=true, Value=nil, Layer!=nil (IsNull() returns true)
//   - Non-null value: Exists=true, Value=<value>, Layer!=nil (HasValue() returns true)
//
// If the value is from a sensitive field and masking is enabled, Masked will be true
// and Value will contain the masked value instead of the original.
type ResolvedValue struct {
	// Value is the resolved value at the path.
	// If Exists is false, this will be nil.
	// If Masked is true, this contains the masked value, not the original.
	Value any

	// Exists indicates whether the key exists at this path.
	// This distinguishes between "key not found" and "key exists with null/zero value".
	Exists bool

	// Layer provides metadata about the layer that provided this value.
	// nil if the value is not set.
	Layer LayerInfo

	// Masked indicates whether the value has been masked for security.
	// When true, Value contains the masked representation, not the original.
	// Use Store.GetAtUnmasked to retrieve the original value.
	Masked bool
}

// IsNull returns true if the key exists but the value is explicitly null.
// This is different from a missing key (Exists=false) and from zero values like 0 or "".
func (rv ResolvedValue) IsNull() bool {
	return rv.Exists && rv.Value == nil
}

// IsMissing returns true if the key does not exist in any layer.
// This is a convenience method equivalent to !Exists.
func (rv ResolvedValue) IsMissing() bool {
	return !rv.Exists
}

// HasValue returns true if the key exists and has a non-null value.
// This includes zero values like 0, "", and false.
func (rv ResolvedValue) HasValue() bool {
	return rv.Exists && rv.Value != nil
}

// newResolvedValue creates a ResolvedValue for the given path from a layer entry.
// Returns an unset ResolvedValue if the entry is nil, data is nil, or path doesn't exist.
func newResolvedValue(entry *layerEntry, path string) ResolvedValue {
	if entry == nil {
		return ResolvedValue{}
	}
	if entry.data == nil {
		return ResolvedValue{}
	}
	value, ok := jsonptr.GetPath(entry.data, path)
	if !ok {
		return ResolvedValue{}
	}
	return ResolvedValue{
		Value:  value,
		Exists: true,
		Layer:  entry,
	}
}

// origin represents the layer entries for a single path,
// sorted by priority (lowest first).
type origin []*layerEntry

// add appends a layer entry to the origin list.
// Entries should be added in priority order (lowest first).
func (o *origin) add(entry *layerEntry) {
	*o = append(*o, entry)
}

// get returns the highest priority layer entry (last element).
// Returns nil if empty.
func (o *origin) get() *layerEntry {
	if o == nil || len(*o) == 0 {
		return nil
	}
	return (*o)[len(*o)-1]
}

// getAll returns all layer entries, sorted by priority (lowest first).
func (o *origin) getAll() []*layerEntry {
	if o == nil {
		return nil
	}
	return *o
}

// origins tracks which layers have values for each path.
// The key is the JSON Pointer path.
type origins struct {
	// leafs maps leaf paths to their origin layer entries.
	leafs map[string]*origin

	// containers maps container paths (maps/slices) to their origin layer entries.
	// This enables fast lookup for container paths in GetAt without full layer traversal.
	containers map[string]*origin
}

// newOrigins creates a new empty origins.
func newOrigins() *origins {
	return &origins{
		leafs:      make(map[string]*origin),
		containers: make(map[string]*origin),
	}
}

// setLeaf appends a layer entry to a leaf path's origin list.
// Entries should be added in priority order (lowest first).
func (o *origins) setLeaf(path string, entry *layerEntry) {
	if o.leafs[path] == nil {
		o.leafs[path] = &origin{}
	}
	o.leafs[path].add(entry)
}

// setContainer appends a layer entry to a container path's origin list.
// Entries should be added in priority order (lowest first).
func (o *origins) setContainer(path string, entry *layerEntry) {
	if o.containers[path] == nil {
		o.containers[path] = &origin{}
	}
	o.containers[path].add(entry)
}

// getLeaf returns the highest priority layer entry for a leaf path.
// Returns nil if not tracked.
func (o *origins) getLeaf(path string) *layerEntry {
	if o.leafs[path] == nil {
		return nil
	}
	return o.leafs[path].get()
}

// getContainer returns the highest priority layer entry for a container path.
// Returns nil if not tracked.
func (o *origins) getContainer(path string) *layerEntry {
	if o.containers[path] == nil {
		return nil
	}
	return o.containers[path].get()
}

// getAllLeaf returns all layer entries for a leaf path, sorted by priority (lowest first).
// Returns nil if not tracked.
func (o *origins) getAllLeaf(path string) []*layerEntry {
	if o.leafs[path] == nil {
		return nil
	}
	return o.leafs[path].getAll()
}

// getAllContainer returns all layer entries for a container path, sorted by priority (lowest first).
// Returns nil if not tracked.
func (o *origins) getAllContainer(path string) []*layerEntry {
	if o.containers[path] == nil {
		return nil
	}
	return o.containers[path].getAll()
}

// isContainer returns true if the path is a known container path.
func (o *origins) isContainer(path string) bool {
	return o.containers[path] != nil
}

// clear removes all entries from the origins.
func (o *origins) clear() {
	clear(o.leafs)
	clear(o.containers)
}

// WalkContext provides access to a configuration path during Walk traversal.
// It allows lazy retrieval of the resolved value or all values from all layers.
//
// The context is valid during the Walk callback. While it can be stored and
// accessed later, the values returned by Value() and AllValues() may reflect
// configuration changes if the store has been reloaded.
type WalkContext struct {
	// Path is the JSON Pointer path for this configuration entry.
	Path string

	origin *origin

	// maskFunc is the function to apply for sensitive values (may be nil)
	maskFunc SensitiveMaskFunc
	// sensitive indicates if this path is sensitive
	sensitive bool
}

// Value returns the resolved value at this path from the highest priority layer.
// If the path is sensitive and masking is enabled, the returned value will be masked.
// Empty values (nil or empty string) are not masked to avoid misleading users.
// Use ValueUnmasked to get the original value.
func (c WalkContext) Value() ResolvedValue {
	entry := c.origin.get()
	rv := newResolvedValue(entry, c.Path)

	// Apply masking if configured and path is sensitive
	// Don't mask empty values (nil or empty string) to avoid misleading users
	if c.maskFunc != nil && c.sensitive && rv.Exists && !isEmptyValue(rv.Value) {
		rv.Value = c.maskFunc(rv.Value)
		rv.Masked = true
	}

	return rv
}

// ValueUnmasked returns the resolved value at this path without masking.
// Use this when you need the actual value for processing.
func (c WalkContext) ValueUnmasked() ResolvedValue {
	entry := c.origin.get()
	return newResolvedValue(entry, c.Path)
}

// IsSensitive returns whether this path is marked as sensitive.
func (c WalkContext) IsSensitive() bool {
	return c.sensitive
}

// AllValues returns all values at this path from all layers that have a value.
// Results are sorted by priority (lowest first).
// This is equivalent to calling Store.GetAllAt(ctx.Path).
func (c WalkContext) AllValues() ResolvedValues {
	entries := c.origin.getAll()
	if len(entries) == 0 {
		return nil
	}

	results := make(ResolvedValues, 0, len(entries))
	for _, entry := range entries {
		if rv := newResolvedValue(entry, c.Path); rv.Exists {
			results = append(results, rv)
		}
	}
	return results
}

// ResolvedValues is a slice of ResolvedValue from multiple layers.
// Values are sorted by priority (lowest first), so the last element
// is the effective value.
type ResolvedValues []ResolvedValue

// Effective returns the highest priority value (the one that takes effect).
// Returns an empty ResolvedValue if the slice is empty.
func (rv ResolvedValues) Effective() ResolvedValue {
	if len(rv) == 0 {
		return ResolvedValue{}
	}
	return rv[len(rv)-1]
}

// Len returns the number of values.
func (rv ResolvedValues) Len() int {
	return len(rv)
}
