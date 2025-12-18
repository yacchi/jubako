// Package env provides an environment variable based configuration layer.
// This layer is read-only and CanSave() returns false.
package env

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/types"
	"github.com/yacchi/jubako/watcher"
)

// TransformFunc transforms environment variable keys and values to JSON Pointer paths.
// It receives the raw key (after prefix removal) and the raw value,
// and returns the JSON Pointer path (e.g., "/server/port") and the final value.
// Return an empty path to skip the variable entirely.
type TransformFunc func(key, value string) (path string, finalValue any)

// EnvironFunc is a function that returns environment variables.
// Each string should be in the format "KEY=value".
// This is typically os.Environ, but can be customized for testing or filtering.
type EnvironFunc func() []string

// DefaultTransform returns the default transform function that converts
// environment variable keys to JSON Pointer paths using the specified delimiter.
//
// The transformation process:
//  1. Convert the key to lowercase
//  2. Split by the delimiter
//  3. Build a JSON Pointer path from the segments
//
// Example with delimiter "_":
//
//	SERVER_PORT -> /server/port
//	DATABASE_HOST -> /database/host
//
// Example with delimiter "__":
//
//	SERVER__PORT -> /server/port
//	MY_APP__LOG_LEVEL -> /my_app/log_level
func DefaultTransform(delim string) TransformFunc {
	return func(key, value string) (string, any) {
		key = strings.ToLower(key)
		segments := strings.Split(key, strings.ToLower(delim))
		escaped := make([]any, len(segments))
		for i, seg := range segments {
			escaped[i] = seg
		}
		return jsonptr.Build(escaped...), value
	}
}

// Option configures the Layer.
type Option func(*Layer)

// WithDelimiter sets a custom delimiter for splitting environment variable keys
// into nested paths. Default is "_".
//
// This also updates the transform function to use the new delimiter.
// If you need a custom transform, call WithTransformFunc after WithDelimiter.
//
// Example with delimiter "__":
//
//	APP_SERVER__HOST=localhost -> /server/host
//	APP_SERVER__PORT=8080      -> /server/port
//
// This is useful when your config keys contain underscores:
//
//	APP_MY_APP__LOG_LEVEL=debug -> /my_app/log_level
func WithDelimiter(delim string) Option {
	return func(l *Layer) {
		l.delim = delim
		l.transform = DefaultTransform(delim)
	}
}

// WithEnvironFunc sets a custom function to provide environment variables.
// Default is os.Environ.
//
// This is useful for:
//   - Testing with controlled environment variables
//   - Pre-filtering variables before processing
//   - Injecting variables from other sources
//
// Example:
//
//	env.New("env", "APP_", env.WithEnvironFunc(func() []string {
//	    return []string{"APP_FOO=bar", "APP_BAZ=qux"}
//	}))
func WithEnvironFunc(fn EnvironFunc) Option {
	return func(l *Layer) {
		l.environ = fn
	}
}

// WithTransformFunc sets a custom transformation function for keys and values.
// The function receives the raw key (after prefix removal) and the raw value,
// and returns the JSON Pointer path and the final value.
//
// Return an empty path to skip the variable entirely.
//
// Note: When using TransformFunc, you have full control over the path transformation.
// The delimiter option is ignored. Use DefaultTransform as a starting point if needed.
//
// Example - skip certain keys:
//
//	env.New("env", "APP_", env.WithTransformFunc(func(key, value string) (string, any) {
//	    if strings.HasPrefix(key, "INTERNAL_") {
//	        return "", nil // skip internal variables
//	    }
//	    // Use default transformation for other keys
//	    return env.DefaultTransform("_")(key, value)
//	}))
//
// Example - custom path mapping:
//
//	env.New("env", "APP_", env.WithTransformFunc(func(key, value string) (string, any) {
//	    // Convert SERVER_HOST to /server/host
//	    key = strings.ToLower(key)
//	    parts := strings.Split(key, "_")
//	    return jsonptr.Build(parts...), value
//	}))
func WithTransformFunc(fn TransformFunc) Option {
	return func(l *Layer) {
		l.transform = fn
	}
}

// Layer loads configuration from environment variables with a specified prefix.
// Environment variable names are converted to configuration paths by:
// 1. Removing the prefix
// 2. Converting to lowercase
// 3. Replacing the delimiter (default "_") with nested paths
//
// Example: with prefix "APP_", environment variable "APP_SERVER_PORT=8080"
// becomes path "/server/port" with value "8080".
//
// With delimiter "__": "APP_SERVER__PORT=8080" becomes "/server/port".
//
// This layer is read-only and does not support saving.
type Layer struct {
	name      layer.Name
	prefix    string
	delim     string
	environ   EnvironFunc
	transform TransformFunc
}

// TypeEnv is the source type identifier for environment variable layers.
const TypeEnv types.SourceType = "env"

// Ensure Layer implements layer.Layer interface (which includes types.DetailsFiller).
var _ layer.Layer = (*Layer)(nil)

// New creates a new environment variable layer.
//
// The prefix is used to filter environment variables. Only variables starting
// with the prefix are included. The prefix is stripped from variable names
// when creating configuration paths.
//
// Example:
//
//	// Load all environment variables starting with "APP_"
//	envLayer := env.New("env", "APP_")
//
//	// With APP_SERVER_PORT=8080 and APP_DATABASE_HOST=localhost:
//	// - /server/port = "8080"
//	// - /database/host = "localhost"
//
// Options can be used to customize behavior:
//
//	// Use double underscore as delimiter
//	env.New("env", "APP_", env.WithDelimiter("__"))
//
//	// Use custom environ function for testing
//	env.New("env", "APP_", env.WithEnvironFunc(myEnvironFunc))
//
//	// Use transform function for custom key mapping
//	env.New("env", "APP_", env.WithTransformFunc(myTransformFunc))
//
// For automatic schema mapping from Store's type T, use NewWithAutoSchema instead.
func New(name layer.Name, prefix string, opts ...Option) *Layer {
	l := &Layer{
		name:      name,
		prefix:    prefix,
		delim:     "_",
		environ:   os.Environ,
		transform: DefaultTransform("_"),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Name returns the layer's name.
func (l *Layer) Name() layer.Name {
	return l.name
}

// Load reads environment variables and returns data as map[string]any.
func (l *Layer) Load(ctx context.Context) (map[string]any, error) {
	// Check for cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data := make(map[string]any)

	for _, env := range l.environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}

		key, value := pair[0], pair[1]

		// Check if the key has our prefix
		if l.prefix != "" && !strings.HasPrefix(key, l.prefix) {
			continue
		}

		// Remove prefix
		key = strings.TrimPrefix(key, l.prefix)

		// Transform key to JSON Pointer path
		path, finalValue := l.transform(key, value)
		if path == "" {
			continue
		}

		// Set the value in the nested map
		jsonptr.SetPath(data, path, finalValue)
	}

	return data, nil
}

// Prefix returns the environment variable prefix.
func (l *Layer) Prefix() string {
	return l.prefix
}

// Delimiter returns the delimiter used for splitting keys into paths.
func (l *Layer) Delimiter() string {
	return l.delim
}

// Save is not supported for environment variable layers.
// Environment variables are inherently read-only in this implementation.
func (l *Layer) Save(ctx context.Context, changeset document.JSONPatchSet) error {
	return fmt.Errorf("environment variable layer %q does not support saving", l.name)
}

// CanSave returns false because environment variable layers are read-only.
func (l *Layer) CanSave() bool {
	return false
}

// FormatEnv is the document format identifier for environment variable layers.
// Environment variables don't have a traditional document format like YAML or JSON,
// but this identifier makes it clear the data comes from environment variables.
const FormatEnv types.DocumentFormat = "env"

// FillDetails populates the Details struct with metadata from this layer.
// Environment variable layers use a noop watcher since environment variables
// are read once at startup.
func (l *Layer) FillDetails(d *types.Details) {
	d.Source = TypeEnv
	d.Format = FormatEnv
	d.Watcher = watcher.TypeNoop
}

// Watch returns a noop LayerWatcher since environment variables are read
// once at startup and don't support watching for changes.
func (l *Layer) Watch(opts ...layer.WatchOption) (layer.LayerWatcher, error) {
	return layer.NewNoopLayerWatcher(), nil
}

// NewWithAutoSchema creates a new environment variable layer that automatically
// builds schema mapping from the Store's configuration type T.
//
// When Store.Add is called, the Store passes its type T via StoreProvider,
// and the schema mapping is built automatically from T's struct tags.
//
// This eliminates the need to specify the type twice (once in Store.New[T] and
// once in env.WithSchemaMapping[T]).
//
// Example:
//
//	type Config struct {
//	    Port int    `json:"port" jubako:"env:SERVER_PORT"`
//	    Host string `json:"host" jubako:"env:SERVER_HOST"`
//	}
//
//	store := jubako.New[Config]()
//	store.Add(env.NewWithAutoSchema("env", "APP_"))
//	// APP_SERVER_PORT=8080 -> /port = 8080
//	// APP_SERVER_HOST=localhost -> /host = "localhost"
func NewWithAutoSchema(name layer.Name, prefix string, opts ...Option) layer.Layer {
	return layer.StoreAwareLayerFunc(func(provider layer.StoreProvider) layer.Layer {
		l := New(name, prefix, opts...)
		if provider != nil {
			schema := buildSchemaMappingFromType(provider.SchemaType(), "")
			l.transform = schema.CreateTransformFunc()
		}
		return l
	})
}
