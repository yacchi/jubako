// Package jubako provides a layered configuration management library.
//
// The name comes from traditional Japanese stacked boxes (重箱) used for
// special occasions. Each layer contains different items, and together they
// form a complete set - much like how this library manages configuration
// from multiple sources.
//
// Key features:
//   - Layer-aware configuration with priority ordering
//   - Format preservation when editing config files (YAML, TOML, JSONC)
//   - Reference-stable containers (Cell pattern)
//   - Type-safe access with generics
//   - Change notification via subscriptions
package jubako
