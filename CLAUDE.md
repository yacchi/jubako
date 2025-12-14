# CLAUDE.md - Jubako Project

## Absolute Rules

- **All documentation, comments, and commit messages MUST be written in English.**
- This rule applies to: code comments, git commit messages (title and body), README, CLAUDE.md, and all other documentation files.
- **Exception (explicitly allowed):** Japanese documentation is permitted only in files whose filenames clearly indicate Japanese content (e.g. `README_ja.md`). All other documentation and all code comments must remain in English.

## Overview

**Jubako** (重箱) is a layered configuration management library for Go.

The name comes from the Japanese traditional stacked boxes used for special occasions. Each layer (tier) contains
different items, and together they form a complete set - just like how this library manages configuration from multiple
sources.

## Design Philosophy

### Common Limitations in Existing Config Libraries

Many configuration libraries share similar limitations:

| Challenge | Common Approach | Jubako's Approach |
|-----------|-----------------|-------------------|
| Layer tracking | Merged into single map (irreversible) | Preserved per layer |
| Write support | Read-only | Read/Write with layer awareness |
| Data structure | `map[string]any` with runtime access | Typed structures + Document AST |
| Comment preservation | Lost on write | Preserved (YAML, TOML, JSONC) |
| Dynamic updates | Callback-only notification | Stable references + notifications |
| Type safety | Runtime assertions | Generics + compile-time checking |
| Reference after reload | Stale pointers | Always current via Cell pattern |

### Core Problems to Solve

1. **Reference Stability**: When config is reloaded, existing references should see updates
2. **Layer Preservation**: Know which layer a value came from, write back to correct layer
3. **Comment Preservation**: Edit config files without losing user comments
4. **Type Safety**: Compile-time type checking, not just runtime

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                          Store[T]                               │
│  ┌──────────────────────┐    ┌──────────────────────┐          │
│  │   Layer Management   │    │  Resolved Values     │          │
│  │                      │    │                      │          │
│  │  layers: []layerEntry│    │  resolved: *Cell[T]  │          │
│  │  origins: origins    │    │  (pointer stable)    │          │
│  └──────────┬───────────┘    └──────────┬───────────┘          │
│             │                           │                       │
│             │      materialize()        │                       │
│             └──────────────────────────>│                       │
│                                         │                       │
│  Subscribe(fn) → unsubscribe func       │                       │
│  Get() T, GetAt(path) ResolvedValue     │                       │
│  SetTo(layer, path, value), Save(layer) │                       │
│  Walk(fn)                               │                       │
└─────────────────────────────────────────┴───────────────────────┘
```

## Key Concepts

### 1. Layer (段)

Each configuration source is a separate layer with priority. Layers are managed through a unified interface:

```go
// layer/layer.go
type Priority int
type Name string

// Layer represents a configuration source that can be loaded and optionally saved.
// Save capability is determined by CanSave(), which delegates to the underlying source.
type Layer interface {
    Name() Name
    Load(ctx context.Context) (map[string]any, error)
    Save(ctx context.Context, changeset document.JSONPatchSet) error
    CanSave() bool
}
```

Priority constants are defined in the root package:

```go
// jubako package
type LayerPriority = layer.Priority

// Step of 10, matching defaultPriorityStep for auto-assigned priorities
const (
    PriorityDefaults LayerPriority = 0  // Lowest
    PriorityUser     LayerPriority = 10
    PriorityProject  LayerPriority = 20
    PriorityEnv      LayerPriority = 30
    PriorityFlags    LayerPriority = 40 // Highest
)
```

### 2. Source and Document (Separation of Concerns)

I/O operations (Source) and format handling (Document) are separated for flexibility:

```go
// source/source.go
type Source interface {
    Load(ctx context.Context) ([]byte, error)
    Save(ctx context.Context, updateFunc UpdateFunc) error
    CanSave() bool
}
```

**Note**: Document is stateless; it parses bytes and can apply patch operations to bytes.

**Implementations**:

- `source/bytes`: Read-only byte slice source
- `source/fs`: File system source with tilde expansion
- `format/yaml`: YAML document with comment preservation
- `format/toml`: TOML document with comment/format preservation
- `format/jsonc`: JSONC document with comment/format preservation

**Security Note**: For configuration files containing sensitive information (credentials, API keys),
use restrictive file permissions:

```go
// Use 0600 for sensitive config files (owner read/write only)
src := fs.New("~/.config/app/secrets.yaml", fs.WithFileMode(0600), fs.WithDirMode(0700))
```

### 3. Document (Interface for AST-based editing)

Abstract interface for format handling and (optionally) comment-preserving edits:

```go
// document/document.go
type Document interface {
    // Parse bytes into map[string]any
    Get(data []byte) (map[string]any, error)

    // Apply changes to bytes (optionally preserving comments/formatting)
    Apply(data []byte, changeset JSONPatchSet) ([]byte, error)

    // Metadata
    Format() DocumentFormat
    CanApply() bool

    // Test helper for generating format-specific test data
    MarshalTestData(data map[string]any) ([]byte, error)
}

type DocumentFormat string

const (
    FormatYAML  DocumentFormat = "yaml"
    FormatTOML  DocumentFormat = "toml"
    FormatJSONC DocumentFormat = "jsonc"
    FormatJSON  DocumentFormat = "json"
)
```

**Supported Implementations**:

- YAML: `format/yaml` using `gopkg.in/yaml.v3` (yaml.Node AST)
- TOML: `format/toml` using `github.com/pelletier/go-toml/v2` (minimal text edits, comment/format preservation)
- JSONC: `format/jsonc` using `github.com/tailscale/hujson` (AST, comment/format preservation)

### 4. Cell[T] (Reactive Value Container)

The key innovation for reference stability:

```go
type Cell[T any] struct {
    value     atomic.Value // stores T
    listeners []listener[T]
    nextID    uint64
    mu        sync.RWMutex
}

func NewCell[T any](initial T) *Cell[T]
func (c *Cell[T]) Get() T                        // Lock-free read
func (c *Cell[T]) Set(v T)                       // Update + notify
func (c *Cell[T]) Subscribe(fn func(T)) func()  // Returns unsubscribe
```

### 5. Origin Tracking

Track which layer each configuration value came from:

```go
// ResolvedValue represents a configuration value with its origin.
type ResolvedValue struct {
    Value  any
    Exists bool
    Layer  LayerInfo  // Metadata about the source layer
}

// LayerInfo provides metadata about a registered layer.
type LayerInfo interface {
    Name() layer.Name
    Priority() layer.Priority
    Format() document.DocumentFormat
    Path() string    // File path (empty for non-file sources)
    Loaded() bool
    ReadOnly() bool  // Whether the layer is marked as read-only
    Writable() bool  // Whether the layer supports saving
    Dirty() bool     // Whether the layer has unsaved changes
}
```

### 6. Materialization (Layer Merging)

Process of merging all layers into resolved config:

1. Sort layers by priority (lower first)
2. Walk each layer's document, recording origin for each path
3. Deep merge maps into single unified map
4. Decode map to typed struct T via JSON
5. Update Cell[T] and notify subscribers

Current merge strategy: **replace** (higher priority completely overrides)

## API Design

### Basic Usage

```go
import (
    "github.com/yacchi/jubako"
    "github.com/yacchi/jubako/layer"
    "github.com/yacchi/jubako/layer/env"
    "github.com/yacchi/jubako/source/bytes"
    "github.com/yacchi/jubako/source/fs"
    "github.com/yacchi/jubako/format/yaml"
)

// Define your config struct
// The materialization process uses JSON for decoding the merged map into your struct,
// so `json` tags are required.
type AppConfig struct {
    Server   ServerConfig `json:"server"`
    Database DBConfig     `json:"database"`
}

// Create store
store := jubako.New[AppConfig]()

// Add layers (priority order: defaults < user < project < env)
store.Add(
    layer.New("defaults", bytes.FromString(defaultsYAML), yaml.New()),
    jubako.WithPriority(jubako.PriorityDefaults),
)

store.Add(
    layer.New("user", fs.New("~/.config/app/config.yaml"), yaml.New()),
    jubako.WithPriority(jubako.PriorityUser),
)

store.Add(
    layer.New("project", fs.New(".app.yaml"), yaml.New()),
    jubako.WithPriority(jubako.PriorityProject),
)

// Environment variables: supports WithDelimiter, WithEnvironFunc, WithTransformFunc options
store.Add(
    env.New("env", "APP_"),
    jubako.WithPriority(jubako.PriorityEnv),
)

// Load and materialize
if err := store.Load(context.Background()); err != nil {
    log.Fatal(err)
}

// Get resolved config (type-safe snapshot)
config := store.Get()
fmt.Println(config.Server.Port)

// Subscribe to changes
unsubscribe := store.Subscribe(func(cfg AppConfig) {
    log.Printf("Config changed: %+v", cfg)
})
defer unsubscribe()

// Write back to specific layer
store.SetTo("user", "/server/port", 9000)
store.SaveLayer(context.Background(), "user")
```

### Origin Tracking

```go
// Get value with origin information
rv := store.GetAt("/server/port")
if rv.Exists {
    fmt.Printf("port=%v (from layer %s)\n", rv.Value, rv.Layer.Name())
}

// Walk all values with their origins
store.Walk(func(ctx jubako.WalkContext) bool {
    rv := ctx.Value()
    fmt.Printf("%s = %v (from %s)\n", ctx.Path, rv.Value, rv.Layer.Name())
    return true
})

// List all layers
for _, info := range store.ListLayers() {
    fmt.Printf("Layer: %s, Priority: %d, Path: %s\n",
        info.Name(), info.Priority(), info.Path())
}
```

### Auto-Priority Mode

```go
// Layers can be added without explicit priority
// They will be assigned priorities in order: 0, 10, 20, ...
store := jubako.New[AppConfig](jubako.WithPriorityStep(10))

store.Add(layer.New("base", bytes.FromString(baseYAML), yaml.New()))
store.Add(layer.New("override", bytes.FromString(overrideYAML), yaml.New()))
```

## Implementation Status

### Phase 1: Core Foundation ✅ Complete

- [x] `Cell[T]` type with atomic operations
- [x] Basic `Store[T]` structure
- [x] Layer priority and ordering

### Phase 2: Document Abstraction ✅ Complete

- [x] `Document` interface
- [x] YAML implementation (`gopkg.in/yaml.v3`)
- [x] Path-based Get/Set/Delete operations
- [x] Comment preservation
- [x] `MarshalTestData` for test data generation

### Phase 3: Layer Management ✅ Complete

- [x] Layer interface (`Layer`)
- [x] `FileLayer` layer implementation
- [x] Source interface and implementations (`bytes`, `fs`)
- [x] Document interface
- [x] Origin tracking (`ResolvedValue`, `LayerInfo`)
- [x] Materialization (merge) logic
- [x] Write-back to correct layer
- [x] Save with comment preservation
- [x] Environment variable layer (`layer/env`)

### Phase 4: Additional Formats ✅ Complete

- [x] TOML support (`pelletier/go-toml/v2`)
- [x] JSONC support (`tailscale/hujson`)
- [ ] Flag integration helpers

### Phase 5: Advanced Features ⏳ Planned

- [ ] Hot reload with file watching
- [ ] Remote config sources (interface)
- [ ] Validation hooks
- [ ] Schema generation
- [ ] Configurable merge strategies via struct tags

## Directory Structure

```
jubako/
├── CLAUDE.md              # This file
├── README.md              # Public documentation
├── go.mod
├── go.sum
├── mise.toml
│
├── jubako.go              # Package documentation
├── cell.go                # Cell[T] implementation
├── cell_test.go
├── store.go               # Main Store[T] type
├── store_test.go
├── layer.go               # Priority constants and type aliases
├── origin.go              # ResolvedValue, origins tracking
├── materialize.go         # Merge logic
├── materialize_test.go
│
├── jsonptr/               # JSONPointer (RFC 6901) utilities
│   ├── jsonptr.go         # Escape, Unescape, Build, Parse
│   └── jsonptr_test.go
│
├── document/              # Document abstraction
│   ├── document.go        # Document interface definition
│   ├── patch.go           # JSONPatch / JSONPatchSet
│   ├── document_test.go
│   ├── errors.go          # Error types
│   └── errors_test.go
│
├── format/                # Format implementations
│   ├── json/              # JSON (encoding/json, no comment preservation)
│   ├── jsonc/             # JSONC (github.com/tailscale/hujson)
│   ├── toml/              # TOML (github.com/pelletier/go-toml/v2)
│   └── yaml/              # YAML (gopkg.in/yaml.v3)
│
├── source/                # Source abstraction
│   ├── source.go          # Source interface
│   ├── bytes/             # Byte slice source
│   │   └── source.go
│   └── fs/                # File system source
│       └── source.go
│
└── layer/                 # Layer implementations
    ├── layer.go           # Layer, FileLayer
    ├── mapdata/           # In-memory map layer (tests/programmatic)
    └── env/               # Environment variable layer
        ├── env.go
        └── env_test.go
```

## Testing Strategy

1. **Unit tests**: Each type (Cell, Document)
2. **Integration tests**: Full layer merge scenarios
3. **Comment preservation tests**: Round-trip editing
4. **Concurrency tests**: Race condition detection
5. **Benchmark**: Measure read performance under concurrent access

## Dependencies

### Optional Dependencies (Phase 4+)

- `gopkg.in/yaml.v3` - YAML support
- `github.com/pelletier/go-toml/v2` - TOML support
- `github.com/tailscale/hujson` - JSONC support
- `github.com/fsnotify/fsnotify` - File watching (Phase 5)

## Design Decisions Log

### Config Structure Definition

**Decision**: Use plain Go structs

```go
// Materialization uses JSON decoding internally, so `json` tags are required.
type AppConfig struct {
    Server   ServerConfig `json:"server"`
    Database DBConfig     `json:"database"`
}

store := jubako.New[AppConfig]()
cfg := store.Get()  // Returns AppConfig snapshot

// Subscribe for changes
store.Subscribe(func(cfg AppConfig) { ... })
```

**Rationale**:

- Low barrier to entry (reuse existing config structs)
- Familiar API for Go developers
- Type-safe access

### Path Query Syntax

**Decision**: JSONPointer (RFC 6901)

```go
// Basic paths
"/server/port"              // server.port
"/servers/0/port"           // servers[0].port
"/config/key.with.dot"      // Keys with dots work naturally

// Escaping (RFC 6901)
// ~0 = ~
// ~1 = /
"/paths/~1api~1users"       // Key is "/api/users"
"/keys/~0tilde"             // Key is "~tilde"
```

**Utility Package**: `jsonptr`

```go
package jsonptr

func Escape(key string) string
func Unescape(key string) string
func Build(keys ...any) string        // Build("server", "hosts", 0) -> "/server/hosts/0"
func Parse(pointer string) ([]string, error)
```

### Merge Strategy

**Current Implementation**: Replace strategy only

- Higher priority completely replaces lower priority
- Maps are deep-merged recursively

**Planned** (Phase 5): Configurable per-field via struct tags

```go
type AppConfig struct {
    Server ServerConfig `yaml:"server"`                        // replace (default)
    Labels map[string]string `yaml:"labels" jubako:"merge:deep"`  // deep merge maps
    Plugins []string `yaml:"plugins" jubako:"merge:append"`       // concatenate slices
}
```

### Why Cell[T] instead of atomic.Value directly?

- Type safety with generics
- Built-in subscription mechanism
- Cleaner API for consumers

### Why separate Source and Document?

- Source handles I/O (files, bytes, network) and optimistic locking
- Document handles format parsing and patch application (YAML, TOML, JSON/JSONC)
- Enables mix-and-match combinations
- Each can be tested independently

### Why Origin tracking?

- Know where each value came from for debugging
- Write back to correct layer
- Support "show effective config" functionality

## Related Projects

- **backlog-cli**: Original project that spawned this library
  - Located at: `../backlog-cli`
  - Current config implementation: `internal/config/`
  - Will migrate to use jubako once stable

## Notes for AI Assistants

When working on this library:

1. Phase 1-3 are complete - focus on Phase 4+ features or bug fixes
2. Write tests alongside implementation
3. Keep the API surface minimal
4. Prioritize correctness over features
5. Document public APIs with examples
6. Follow existing code patterns

The key innovation is **reference stability** through the Cell pattern. Users should be able to:

```go
store := jubako.New[AppConfig]()
// ... setup layers ...
store.Load(ctx)

config := store.Get() // Get once
// ... later, even after reload ...
store.Reload(ctx)
newConfig := store.Get() // Still works, returns latest
```
