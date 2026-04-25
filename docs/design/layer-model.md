# Layer Model

## Purpose

Jubako is designed around the idea that a configuration source should remain visible as a layer even after the final
configuration is resolved.

Many configuration systems stop at "load and merge". Jubako keeps more structure so the system can answer additional
questions later:

- which layer produced the effective value?
- which layer should receive a write?
- can this layer be watched?
- is this layer optional, sensitive, or read-only?

## Core abstractions

### `Source`

`Source` owns I/O. It loads raw bytes and, when writable, persists updates with optimistic locking semantics.

### `Document`

`Document` owns format semantics. It parses bytes into `map[string]any` and, when supported, applies a patch set back
to the original bytes without regenerating the whole document.

### `Layer`

`Layer` is the unit the `Store` sees. A layer loads logical configuration as `map[string]any`, can optionally save a
change set, and can optionally expose watch support.

This makes the public composition seam explicit:

- `Source` handles storage
- `Document` handles syntax and patching
- `Layer` handles logical configuration boundaries

## Why the `Store` works with layers

The `Store` intentionally does not merge at the source level or the document level.

That would make it difficult to preserve:

- per-layer metadata
- origin tracking
- save routing
- watch behavior
- future coordinated layers that project one logical view onto multiple physical stores

## Layer metadata

At add time, the store attaches policy and runtime metadata to each layer:

- priority
- read-only
- no-watch
- sensitive
- optional

These are store-level concerns. They describe how the layer participates in the resolved configuration, even if the
underlying source itself is technically writable or watchable.

## Schema-aware extensions

The store also builds schema information from `T` once and exposes a read-only `SchemaView` to layers that need it.

This allows higher-level layers to inspect:

- resolved paths
- field sensitivity
- raw struct tags
- reflected field metadata

The important constraint is that this is read-only. Layers can inspect schema metadata, but the store keeps ownership
of schema construction and mutation rules.
