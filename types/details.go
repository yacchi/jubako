// Package types provides common type definitions shared across jubako packages.
// This package contains only type definitions and interfaces, no logic.
package types

// SourceType identifies the type of a configuration source.
// Constants for standard types are defined in the source package.
type SourceType string

// DocumentFormat identifies the format of a configuration document.
// Constants for standard formats are defined in the document package.
type DocumentFormat string

// WatcherType identifies the type of a watcher.
// Constants for standard types are defined in the watcher package.
type WatcherType string

// Details holds metadata about a layer and its underlying source.
// It provides a unified way to access layer information without
// requiring multiple type assertions.
type Details struct {
	// Source is the type of source (e.g., "fs", "bytes", "s3", "ssm").
	// Empty if not specified by the source.
	Source SourceType

	// Path is the file path for file-based sources (empty for non-file sources).
	Path string

	// Format is the document format (e.g., "yaml", "toml", "json").
	// Empty for layers that don't use a document format.
	Format DocumentFormat

	// Watcher is the type of watcher used for change detection
	// (e.g., "polling", "subscription", "noop").
	// Empty if the source does not support watching.
	Watcher WatcherType
}

// DetailsFiller is an interface for populating Details with metadata.
// Both layers and sources can implement this interface to contribute
// their respective metadata.
type DetailsFiller interface {
	FillDetails(d *Details)
}
