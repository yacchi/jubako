# Jubako

**Jubako** (重箱) is a layered configuration management library for Go.

The name comes from traditional Japanese stacked boxes used for special occasions. Each layer (tier) contains different
items, and together they form a complete set - much like how this library manages configuration from multiple sources.

[日本語版 README](README_ja.md)

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
  - [Layers](#layers)
  - [JSON Pointer (RFC 6901)](#json-pointer-rfc-6901)
  - [Config Struct Definition](#config-struct-definition)
- [API Reference](#api-reference)
  - [Store[T]](#storet)
  - [Origin Tracking](#origin-tracking)
  - [Layer Information](#layer-information)
- [Supported Formats](#supported-formats)
  - [Environment Variable Layer](#environment-variable-layer)
- [Custom Format and Source Implementation](#custom-format-and-source-implementation)
  - [Source Interface](#source-interface)
  - [Parser Interface](#parser-interface)
  - [Document Interface](#document-interface)
  - [Simple Implementation with mapdoc](#simple-implementation-with-mapdoc)
  - [Layer Interface](#layer-interface)
- [Comparison with Other Libraries](#comparison-with-other-libraries)
- [License](#license)
- [Contributing](#contributing)

## Features

- **Layer-aware configuration** - Manage multiple config sources with priority ordering
- **Origin tracking** - Track which layer each configuration value comes from
- **Format preservation** - AST-based processing updates only changed values (preserves comments, whitespace, indentation, etc.)
- **Type-safe access** - Generics-based API with compile-time type checking
- **Change notifications** - Subscribe to configuration changes

## Installation

```bash
go get github.com/yacchi/jubako
```

**Requirements:** Go 1.24+

### Optional format modules

Additional formats are provided as separate Go modules so their dependencies don’t become requirements of the core library:

```bash
go get github.com/yacchi/jubako/format/yaml
go get github.com/yacchi/jubako/format/toml
go get github.com/yacchi/jubako/format/jsonc
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/yacchi/jubako"
    "github.com/yacchi/jubako/format/yaml"
    "github.com/yacchi/jubako/layer"
    "github.com/yacchi/jubako/layer/env"
    "github.com/yacchi/jubako/source/bytes"
    "github.com/yacchi/jubako/source/fs"
)

type AppConfig struct {
    Server   ServerConfig   `yaml:"server" json:"server"`
    Database DatabaseConfig `yaml:"database" json:"database"`
}

type ServerConfig struct {
    Host string `yaml:"host" json:"host"`
    Port int    `yaml:"port" json:"port"`
}

type DatabaseConfig struct {
    URL string `yaml:"url" json:"url"`
}

const defaultsYAML = `
server:
  host: localhost
  port: 8080
database:
  url: postgres://localhost/myapp
`

func main() {
    ctx := context.Background()

    // Create a new store
    store := jubako.New[AppConfig]()

    // Add configuration layers (lower priority first)
    store.Add(
        layer.New("defaults", bytes.FromString(defaultsYAML), yaml.NewParser()),
        jubako.WithPriority(jubako.PriorityDefaults),
    )

    store.Add(
        layer.New("user", fs.New("~/.config/app/config.yaml"), yaml.NewParser()),
        jubako.WithPriority(jubako.PriorityUser),
    )

    store.Add(
        layer.New("project", fs.New(".app.yaml"), yaml.NewParser()),
        jubako.WithPriority(jubako.PriorityProject),
    )

    store.Add(
        env.New("env", "APP_"),
        jubako.WithPriority(jubako.PriorityEnv),
    )

    // Load and materialize configuration
    if err := store.Load(ctx); err != nil {
        log.Fatal(err)
    }

    // Get the resolved configuration
    config := store.Get()
    fmt.Printf("Server: %s:%d\n", config.Server.Host, config.Server.Port)

    // Subscribe to changes
    unsubscribe := store.Subscribe(func(cfg AppConfig) {
        log.Printf("Config changed: %+v", cfg)
    })
    defer unsubscribe()
}
```

For complete working examples, see the [examples/](examples/) directory:

- [basic](examples/basic/) - Basic usage (adding layers, loading, modifying, saving)
- [env-override](examples/env-override/) - Environment variable overrides
- [origin-tracking](examples/origin-tracking/) - Detailed origin tracking features

## Core Concepts

### Layers

Each configuration source is represented as a layer with a priority. Higher priority layers override values from lower
priority layers.

```go
const (
    PriorityDefaults LayerPriority = 0  // Lowest - default values
    PriorityUser     LayerPriority = 10 // User-level config (~/.config)
    PriorityProject  LayerPriority = 20 // Project-level config (.app.yaml)
    PriorityEnv      LayerPriority = 30 // Environment variables
    PriorityFlags    LayerPriority = 40 // Highest - command-line flags
)
```

**Priority Ordering Example:**

```
┌─────────────────────┐
│   Command Flags     │ ← Priority 40 (Highest)
├─────────────────────┤
│   Environment Vars  │ ← Priority 30
├─────────────────────┤
│   Project Config    │ ← Priority 20
├─────────────────────┤
│   User Config       │ ← Priority 10
├─────────────────────┤
│   Defaults          │ ← Priority 0 (Lowest)
└─────────────────────┘
```

### JSON Pointer (RFC 6901)

Jubako uses JSON Pointer for path-based configuration access:

```go
import "github.com/yacchi/jubako/jsonptr"

// Build a pointer
ptr1 := jsonptr.Build("server", "port")     // "/server/port"
ptr2 := jsonptr.Build("servers", 0, "name") // "/servers/0/name"

// Parse a pointer
keys, _ := jsonptr.Parse("/server/port")     // ["server", "port"]

// Handle special characters
ptr3 := jsonptr.Build("feature.flags", "on/off") // "/feature.flags/on~1off"
```

**Escaping Rules (RFC 6901):**

- `~` is encoded as `~0`
- `/` is encoded as `~1`

### Config Struct Definition

When defining config structs, specify both `yaml` and `json` tags.
The materialization process uses JSON internally to decode the merged map into your struct,
so `json` tags are required.

```go
type AppConfig struct {
    Server   ServerConfig   `yaml:"server" json:"server"`
    Database DatabaseConfig `yaml:"database" json:"database"`
}

type ServerConfig struct {
    Host string `yaml:"host" json:"host"`
    Port int    `yaml:"port" json:"port"`
}
```

## API Reference

### Store[T]

Store is the central type for configuration management.

#### Creation and Options

```go
// Create a new store
store := jubako.New[AppConfig]()

// Specify auto-priority step (default: 10)
storeWithStep := jubako.New[AppConfig](jubako.WithPriorityStep(100))
```

#### Adding Layers

```go
// Add layer with explicit priority
err := store.Add(
    layer.New("defaults", bytes.FromString(defaultsYAML), yaml.NewParser()),
    jubako.WithPriority(jubako.PriorityDefaults),
)

// Add as read-only (prevents modifications via SetTo)
err = store.Add(
    layer.New("system", fs.New("/etc/app/config.yaml"), yaml.NewParser()),
    jubako.WithPriority(jubako.PriorityDefaults),
    jubako.WithReadOnly(),
)

// Without priority, auto-assigned in order (0, 10, 20, ...)
store.Add(layer.New("base", bytes.FromString(baseYAML), yaml.NewParser()))
store.Add(layer.New("override", bytes.FromString(overrideYAML), yaml.NewParser()))
```

#### Loading and Access

```go
// Load all layers
err := store.Load(ctx)

// Reload configuration
err = store.Reload(ctx)

// Get merged configuration
config := store.Get()
fmt.Println(config.Server.Port)
```

#### Change Notifications

```go
// Subscribe to configuration changes
unsubscribe := store.Subscribe(func(cfg AppConfig) {
    log.Printf("Config changed: %+v", cfg)
})
defer unsubscribe()
```

#### Modifying and Saving

```go
// Modify value in specific layer (in memory)
err := store.SetTo("user", "/server/port", 9000)

// Check for unsaved changes
if store.IsDirty() {
    // Save all modified layers
    err = store.Save(ctx)

    // Or save specific layer only
    err = store.SaveLayer(ctx, "user")
}
```

### Origin Tracking

Track which layer each configuration value comes from.

#### GetAt - Get Single Value

```go
rv := store.GetAt("/server/port")
if rv.Exists {
    fmt.Printf("port=%v (from layer %s)\n", rv.Value, rv.Layer.Name())
}
```

#### GetAllAt - Get Values from All Layers

```go
values := store.GetAllAt("/server/port")
for _, rv := range values {
    fmt.Printf("port=%v (from layer %s, priority %d)\n",
        rv.Value, rv.Layer.Name(), rv.Layer.Priority())
}

// Get the highest priority value
effective := values.Effective()
fmt.Printf("effective: %v\n", effective.Value)
```

#### Walk - Traverse All Values

```go
// Get resolved value for each path
store.Walk(func(ctx jubako.WalkContext) bool {
    rv := ctx.Value()
    fmt.Printf("%s = %v (from %s)\n", ctx.Path, rv.Value, rv.Layer.Name())
    return true // continue
})

// Get all layer values for each path (analyze override chain)
store.Walk(func(ctx jubako.WalkContext) bool {
    allValues := ctx.AllValues()
    if allValues.Len() > 1 {
        fmt.Printf("%s has values from %d layers:\n", ctx.Path, allValues.Len())
        for _, rv := range allValues {
            fmt.Printf("  - %s: %v\n", rv.Layer.Name(), rv.Value)
        }
    }
    return true
})
```

See [examples/origin-tracking](examples/origin-tracking/) for detailed usage.

### Layer Information

```go
// Get specific layer info
info := store.GetLayerInfo("user")
if info != nil {
    fmt.Printf("Name: %s\n", info.Name())
    fmt.Printf("Priority: %d\n", info.Priority())
    fmt.Printf("Format: %s\n", info.Format())
    fmt.Printf("Path: %s\n", info.Path())       // for file-based layers
    fmt.Printf("Loaded: %v\n", info.Loaded())
    fmt.Printf("ReadOnly: %v\n", info.ReadOnly())
    fmt.Printf("Writable: %v\n", info.Writable())
    fmt.Printf("Dirty: %v\n", info.Dirty())
}

// List all layers (sorted by priority)
for _, info := range store.ListLayers() {
    fmt.Printf("[%d] %s (writable: %v)\n",
        info.Priority(), info.Name(), info.Writable())
}
```

## Supported Formats

Jubako supports two types of format implementations:

### Full Support Formats (Format Preservation)

These formats update only changed values while preserving the original format including comments,
blank lines, indentation, and key ordering.

| Format | Package | Description |
|--------|---------|-------------|
| YAML | `format/yaml` | Uses yaml.Node AST from `gopkg.in/yaml.v3` |
| TOML | `format/toml` | Preserves comments/format via minimal text edits |
| JSONC | `format/jsonc` | Preserves comments/format via hujson AST |

```go
// YAML (with comment preservation)
store.Add(
    layer.New("user", fs.New("~/.config/app.yaml"), yaml.NewParser()),
    jubako.WithPriority(jubako.PriorityUser),
)
```

With full support formats, the original format is preserved when modifying values:

```yaml
# User settings

server:
  port: 8080  # Custom port

# ↑ After store.SetTo("user", "/server/port", 9000),
# comments, blank lines, and indentation remain intact
```

### Simple Support Formats (mapdoc-based)

Simple implementations backed by `map[string]any`. Comments are not preserved,
but reading and writing works correctly.

| Format | Package | Description |
|--------|---------|-------------|
| JSON | `format/json` | Uses standard library `encoding/json` |

```go
// JSON (without comment preservation)
store.Add(
    layer.New("config", fs.New("config.json"), json.NewParser()),
    jubako.WithPriority(jubako.PriorityProject),
)
```

### Summary

| Source | How to add | Format Preservation |
|--------|------------|---------------------|
| YAML | `layer.New(..., <source>, yaml.NewParser())` | Yes |
| TOML | `layer.New(..., <source>, toml.NewParser())` | Yes |
| JSONC | `layer.New(..., <source>, jsonc.NewParser())` | Yes |
| JSON | `layer.New(..., <source>, json.NewParser())` | No |
| Environment variables | `env.New(name, prefix)` | N/A |

### Environment Variable Layer

The environment variable layer reads environment variables with a matching prefix:

```go
// Read environment variables with APP_ prefix
// APP_SERVER_HOST -> /server/host
// APP_DATABASE_USER -> /database/user
store.Add(
    env.New("env", "APP_"),
    jubako.WithPriority(jubako.PriorityEnv),
)
```

**Note**: Environment variables are always read as strings. For numeric fields,
consider using them in combination with YAML layers.

See [examples/env-override](examples/env-override/) for detailed usage.

## Custom Format and Source Implementation

Jubako has an extensible architecture. You can implement custom formats and sources.

### Source Interface

Source handles configuration data I/O (format-agnostic):

```go
// source/source.go
type Source interface {
    // Load reads configuration data from the source
    Load(ctx context.Context) ([]byte, error)

    // Save writes data to the source
    // Returns ErrSaveNotSupported if saving is not supported
    Save(ctx context.Context, data []byte) error

    // CanSave returns whether saving is supported
    CanSave() bool
}
```

**Example Implementation (HTTP Source)**:

```go
type HTTPSource struct {
    url string
}

func NewHTTP(url string) *HTTPSource {
    return &HTTPSource{url: url}
}

func (s *HTTPSource) Load(ctx context.Context) ([]byte, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", s.url, nil)
    if err != nil {
        return nil, err
    }
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    return io.ReadAll(resp.Body)
}

func (s *HTTPSource) Save(ctx context.Context, data []byte) error {
    return source.ErrSaveNotSupported
}

func (s *HTTPSource) CanSave() bool {
    return false
}

// Usage
store.Add(
    layer.New("remote", NewHTTP("https://config.example.com/app.yaml"), yaml.NewParser()),
    jubako.WithPriority(jubako.PriorityDefaults),
)
```

### Parser Interface

Parser converts raw bytes into a Document:

```go
// document/parser.go
type Parser interface {
    // Parse converts bytes to Document
    Parse(data []byte) (Document, error)

    // Format returns the format this parser handles
    Format() DocumentFormat

    // CanMarshal returns whether marshaling with comment preservation is supported
    CanMarshal() bool
}
```

### Document Interface

Document provides access to structured configuration data:

```go
// document/document.go
type Document interface {
    // Get retrieves value at path (JSON Pointer)
    Get(path string) (any, bool)

    // Set sets value at path
    Set(path string, value any) error

    // Delete removes value at path
    Delete(path string) error

    // Marshal serializes document to bytes
    // Preserves comments and formatting where possible
    Marshal() ([]byte, error)

    // Format returns the document format
    Format() DocumentFormat

    // MarshalTestData converts data to bytes for testing
    MarshalTestData(data map[string]any) ([]byte, error)
}
```

### Two Approaches for Format Implementation

When implementing custom formats, there are two approaches:

#### 1. Simple Implementation with mapdoc

When format preservation is not needed, use the `mapdoc` package to easily add formats.
It provides all basic features including JSON Pointer path access and automatic intermediate map creation,
backed by `map[string]any`.

**JSON format implementation example (~30 lines)**:

```go
// format/json/document.go
package json

import (
    "encoding/json"
    "fmt"

    "github.com/yacchi/jubako/document"
    "github.com/yacchi/jubako/mapdoc"
)

// Document is a JSON document backed by map[string]any
type Document = mapdoc.Document

// Parse converts JSON data to a document
func Parse(data []byte) (*Document, error) {
    var root map[string]any
    if err := json.Unmarshal(data, &root); err != nil {
        return nil, fmt.Errorf("failed to parse JSON: %w", err)
    }
    return mapdoc.New(
        document.FormatJSON,
        mapdoc.WithData(root),
        mapdoc.WithMarshal(marshalJSON),
    ), nil
}

func marshalJSON(data map[string]any) ([]byte, error) {
    return json.MarshalIndent(data, "", "  ")
}
```

#### 2. AST-based Full Support Implementation

When comment and format preservation is required, implement a Document that directly
manipulates the format-specific AST.

**YAML format implementation overview**:

```go
// format/yaml/document.go
package yaml

import (
    "gopkg.in/yaml.v3"
    "github.com/yacchi/jubako/document"
)

// Document is a YAML document backed by yaml.Node AST
type Document struct {
    root *yaml.Node  // Holds the AST directly
}

// Get traverses yaml.Node to retrieve values
func (d *Document) Get(path string) (any, bool) {
    // Search nodes in AST and convert to values
}

// Set traverses and updates yaml.Node
func (d *Document) Set(path string, value any) error {
    // Update existing nodes or create new ones
    // Comments are attached to existing nodes, so they're preserved
}

// Marshal serializes the AST as-is
func (d *Document) Marshal() ([]byte, error) {
    return yaml.Marshal(d.root)  // Outputs with comments
}
```

TOML and JSONC achieve comment preservation similarly by manipulating their respective library ASTs.

### Layer Interface

Layer represents a configuration source combining Source and Parser.
Typically use `SourceParser` created by `layer.New()`, but special implementations
like the environment variable layer are also possible:

```go
// layer/layer.go
type Layer interface {
    // Name returns the unique identifier for this layer
    Name() Name

    // Load loads configuration and returns a Document
    Load(ctx context.Context) (Document, error)

    // Document returns the loaded Document
    Document() Document

    // Save persists the Document to the source
    Save(ctx context.Context) error

    // CanSave returns whether saving is supported
    CanSave() bool
}
```

See the following packages for existing implementations:

- `source/bytes` - Byte slice source (read-only)
- `source/fs` - File system source
- `format/yaml` - YAML parser (AST-based, comment preservation)
- `format/toml` - TOML parser (separate module, comment + format preservation)
- `format/jsonc` - JSONC parser (separate module, comment + format preservation)
- `format/json` - JSON parser (mapdoc-based, simple)
- `layer/env` - Environment variable layer

## Comparison with Typical Config Libraries

| Feature | Jubako | Typical Libraries |
|---------|--------|-------------------|
| Layer tracking | Per-layer preservation | Merged (irreversible) |
| Origin tracking | Yes | No |
| Write support | Layer-aware write-back | Limited |
| Format preservation | Yes (AST-based, supported formats) | No |

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
