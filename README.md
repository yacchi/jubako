# Jubako

[![CI](https://github.com/yacchi/jubako/actions/workflows/ci.yml/badge.svg)](https://github.com/yacchi/jubako/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/yacchi/jubako/graph/badge.svg)](https://codecov.io/gh/yacchi/jubako)
[![Go Reference](https://pkg.go.dev/badge/github.com/yacchi/jubako.svg)](https://pkg.go.dev/github.com/yacchi/jubako)

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
- **Format preservation** - AST-based processing updates only changed values (preserves comments, whitespace,
  indentation, etc.)
- **Type-safe access** - Generics-based API with compile-time type checking
- **Change notifications** - Subscribe to configuration changes

## Installation

```bash
go get github.com/yacchi/jubako
```

**Requirements:** Go 1.24+

### Optional format modules

Additional formats are provided as separate Go modules so their dependencies don’t become requirements of the core
library:

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
	if err := store.Add(
		layer.New("defaults", bytes.FromString(defaultsYAML), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityDefaults),
	); err != nil {
		log.Fatal(err)
	}

	if err := store.Add(
		layer.New("user", fs.New("~/.config/app/config.yaml"), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityUser),
	); err != nil {
		log.Fatal(err)
	}

	if err := store.Add(
		layer.New("project", fs.New(".app.yaml"), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityProject),
	); err != nil {
		log.Fatal(err)
	}

	if err := store.Add(
		env.New("env", "APP_"),
		jubako.WithPriority(jubako.PriorityEnv),
	); err != nil {
		log.Fatal(err)
	}

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
package main

import "github.com/yacchi/jubako"

func main() {
	_ = jubako.PriorityDefaults // 0: lowest
	_ = jubako.PriorityUser     // 10
	_ = jubako.PriorityProject  // 20
	_ = jubako.PriorityEnv      // 30
	_ = jubako.PriorityFlags    // 40: highest
}
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
package main

import "github.com/yacchi/jubako/jsonptr"

func main() {
	// Build a pointer
	ptr1 := jsonptr.Build("server", "port")     // "/server/port"
	ptr2 := jsonptr.Build("servers", 0, "name") // "/servers/0/name"

	// Parse a pointer
	keys, _ := jsonptr.Parse("/server/port") // ["server", "port"]

	// Handle special characters
	ptr3 := jsonptr.Build("feature.flags", "on/off") // "/feature.flags/on~1off"

	_ = ptr1
	_ = ptr2
	_ = ptr3
	_ = keys
}
```

**Escaping Rules (RFC 6901):**

- `~` is encoded as `~0`
- `/` is encoded as `~1`

### Config Struct Definition

When defining config structs, `json` tags are required.
The materialization process uses `encoding/json` internally to decode the merged map into your struct.
Add format-specific tags such as `yaml` or `toml` as needed.

```go
package main

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
```

## API Reference

### Store[T]

Store is the central type for configuration management.

#### Creation and Options

```go
package main

import "github.com/yacchi/jubako"

type AppConfig struct{}

func main() {
	// Create a new store
	store := jubako.New[AppConfig]()

	// Specify auto-priority step (default: 10)
	storeWithStep := jubako.New[AppConfig](jubako.WithPriorityStep(100))

	_ = store
	_ = storeWithStep
}
```

#### Adding Layers

```go
package main

import (
	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/format/yaml"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source/bytes"
	"github.com/yacchi/jubako/source/fs"
)

type AppConfig struct{}

const (
	defaultsYAML = ""
	baseYAML     = ""
	overrideYAML = ""
)

func main() {
	store := jubako.New[AppConfig]()

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
	err = store.Add(layer.New("base", bytes.FromString(baseYAML), yaml.NewParser()))
	err = store.Add(layer.New("override", bytes.FromString(overrideYAML), yaml.NewParser()))

	_ = err
}
```

#### Loading and Access

```go
package main

import (
	"context"
	"fmt"

	"github.com/yacchi/jubako"
)

type AppConfig struct {
	Server struct {
		Port int `json:"port"`
	} `json:"server"`
}

func main() {
	ctx := context.Background()
	store := jubako.New[AppConfig]()

	// Load all layers
	err := store.Load(ctx)

	// Reload configuration
	err = store.Reload(ctx)

	// Get merged configuration
	config := store.Get()
	fmt.Println(config.Server.Port)

	_ = err
}
```

#### Change Notifications

```go
package main

import (
	"log"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	// Subscribe to configuration changes
	unsubscribe := store.Subscribe(func(cfg AppConfig) {
		log.Printf("Config changed: %+v", cfg)
	})
	defer unsubscribe()
}
```

#### Modifying and Saving

```go
package main

import (
	"context"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	ctx := context.Background()
	store := jubako.New[AppConfig]()

	// Modify value in specific layer (in memory)
	err := store.SetTo("user", "/server/port", 9000)

	// Check for unsaved changes
	if store.IsDirty() {
		// Save all modified layers
		err = store.Save(ctx)

		// Or save specific layer only
		err = store.SaveLayer(ctx, "user")
	}

	_ = err
}
```

### Origin Tracking

Track which layer each configuration value comes from.

#### GetAt - Get Single Value

```go
package main

import (
	"fmt"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	rv := store.GetAt("/server/port")
	if rv.Exists {
		fmt.Printf("port=%v (from layer %s)\n", rv.Value, rv.Layer.Name())
	}
}
```

#### GetAllAt - Get Values from All Layers

```go
package main

import (
	"fmt"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	values := store.GetAllAt("/server/port")
	for _, rv := range values {
		fmt.Printf("port=%v (from layer %s, priority %d)\n",
			rv.Value, rv.Layer.Name(), rv.Layer.Priority())
	}

	// Get the highest priority value
	effective := values.Effective()
	fmt.Printf("effective: %v\n", effective.Value)
}
```

#### Walk - Traverse All Values

```go
package main

import (
	"fmt"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

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
}
```

See [examples/origin-tracking](examples/origin-tracking/) for detailed usage.

### Layer Information

```go
package main

import (
	"fmt"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	// Get specific layer info
	info := store.GetLayerInfo("user")
	if info != nil {
		fmt.Printf("Name: %s\n", info.Name())
		fmt.Printf("Priority: %d\n", info.Priority())
		fmt.Printf("Format: %s\n", info.Format())
		fmt.Printf("Path: %s\n", info.Path()) // for file-based layers
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
}
```

## Supported Formats

Jubako supports two types of format implementations:

### Full Support Formats (Format Preservation)

These formats update only changed values while preserving the original format including comments,
blank lines, indentation, and key ordering.

| Format | Package        | Description                                      |
|--------|----------------|--------------------------------------------------|
| YAML   | `format/yaml`  | Uses yaml.Node AST from `gopkg.in/yaml.v3`       |
| TOML   | `format/toml`  | Preserves comments/format via minimal text edits |
| JSONC  | `format/jsonc` | Preserves comments/format via hujson AST         |

```go
package main

import (
	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/format/yaml"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source/fs"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	// YAML (with comment preservation)
	_ = store.Add(
		layer.New("user", fs.New("~/.config/app.yaml"), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityUser),
	)
}
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

| Format | Package       | Description                           |
|--------|---------------|---------------------------------------|
| JSON   | `format/json` | Uses standard library `encoding/json` |

```go
package main

import (
	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/format/json"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source/fs"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	// JSON (without comment preservation)
	_ = store.Add(
		layer.New("config", fs.New("config.json"), json.NewParser()),
		jubako.WithPriority(jubako.PriorityProject),
	)
}
```

### Summary

| Source                | How to add                                    | Format Preservation |
|-----------------------|-----------------------------------------------|---------------------|
| YAML                  | `layer.New(..., <source>, yaml.NewParser())`  | Yes                 |
| TOML                  | `layer.New(..., <source>, toml.NewParser())`  | Yes                 |
| JSONC                 | `layer.New(..., <source>, jsonc.NewParser())` | Yes                 |
| JSON                  | `layer.New(..., <source>, json.NewParser())`  | No                  |
| Environment variables | `env.New(name, prefix)`                       | N/A                 |

### Environment Variable Layer

The environment variable layer reads environment variables with a matching prefix:

```go
package main

import (
	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/layer/env"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	// Read environment variables with APP_ prefix
	// APP_SERVER_HOST -> /server/host
	// APP_DATABASE_USER -> /database/user
	_ = store.Add(
		env.New("env", "APP_"),
		jubako.WithPriority(jubako.PriorityEnv),
	)
}
```

**Note**: Environment variables are always read as strings. For numeric fields,
consider using them in combination with YAML layers.

See [examples/env-override](examples/env-override/) for detailed usage.

## Custom Format and Source Implementation

Jubako has an extensible architecture. You can implement custom formats and sources.

### Source Interface

Source handles configuration data I/O (format-agnostic):

```go
package source

import "context"

// source/source.go
type Source interface {
	// Load reads configuration data from the source.
	Load(ctx context.Context) ([]byte, error)

	// Save writes data to the source.
	// Returns ErrSaveNotSupported if saving is not supported.
	Save(ctx context.Context, data []byte) error

	// CanSave returns whether saving is supported.
	CanSave() bool
}
```

**Example Implementation (HTTP Source)**:

```go
package main

import (
	"context"
	"io"
	"net/http"

	"github.com/yacchi/jubako/source"
)

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

// Usage:
//
//  store.Add(
//      layer.New("remote", NewHTTP("https://config.example.com/app.yaml"), yaml.NewParser()),
//      jubako.WithPriority(jubako.PriorityDefaults),
//  )
```

### Parser Interface

Parser converts raw bytes into a Document:

```go
package document

type Document interface{}
type DocumentFormat string

// document/parser.go
type Parser interface {
	// Parse converts bytes to Document.
	Parse(data []byte) (Document, error)

	// Format returns the format this parser handles.
	Format() DocumentFormat

	// CanMarshal returns whether marshaling with comment preservation is supported.
	CanMarshal() bool
}
```

### Document Interface

Document provides access to structured configuration data:

```go
package document

type DocumentFormat string

// document/document.go
type Document interface {
	// Get retrieves value at path (JSON Pointer).
	Get(path string) (any, bool)

	// Set sets value at path.
	Set(path string, value any) error

	// Delete removes value at path.
	Delete(path string) error

	// Marshal serializes document to bytes.
	// Preserves comments and formatting where possible.
	Marshal() ([]byte, error)

	// Format returns the document format.
	Format() DocumentFormat

	// MarshalTestData converts data to bytes for testing.
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
	"fmt"

	"github.com/yacchi/jubako/document"
	"gopkg.in/yaml.v3"
)

// Document is a YAML document backed by yaml.Node AST
type Document struct {
	root *yaml.Node // Holds the AST directly
}

var _ document.Document = (*Document)(nil)

// Get traverses yaml.Node to retrieve values.
func (d *Document) Get(path string) (any, bool) { return nil, false }

// Set traverses and updates yaml.Node.
func (d *Document) Set(path string, value any) error { return nil }

// Delete removes the value at path.
func (d *Document) Delete(path string) error { return nil }

// Marshal serializes the AST as-is.
func (d *Document) Marshal() ([]byte, error) {
	return yaml.Marshal(d.root) // Outputs with comments
}

// Format returns the document format.
func (d *Document) Format() document.DocumentFormat { return document.FormatYAML }

// MarshalTestData generates YAML bytes for tests.
func (d *Document) MarshalTestData(data map[string]any) ([]byte, error) {
	if data == nil {
		data = map[string]any{}
	}
	b, err := yaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal test data: %w", err)
	}
	return b, nil
}
```

TOML and JSONC achieve comment preservation similarly by manipulating their respective library ASTs.

### Layer Interface

Layer represents a configuration source combining Source and Parser.
Typically use `SourceParser` created by `layer.New()`, but special implementations
like the environment variable layer are also possible:

```go
package layer

import (
	"context"

	"github.com/yacchi/jubako/document"
)

type Name string

// layer/layer.go
type Layer interface {
	// Name returns the unique identifier for this layer.
	Name() Name

	// Load loads configuration and returns a Document.
	Load(ctx context.Context) (document.Document, error)

	// Document returns the loaded Document.
	Document() document.Document

	// Save persists the Document to the source.
	Save(ctx context.Context) error

	// CanSave returns whether saving is supported.
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

| Feature             | Jubako                             | Typical Libraries     |
|---------------------|------------------------------------|-----------------------|
| Layer tracking      | Per-layer preservation             | Merged (irreversible) |
| Origin tracking     | Yes                                | No                    |
| Write support       | Layer-aware write-back             | Limited               |
| Format preservation | Yes (AST-based, supported formats) | No                    |

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
