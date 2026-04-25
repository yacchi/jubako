# Write Path

## Goal

The write path is designed so that Jubako can modify configuration without losing the layer boundary that originally
made the value visible.

In practice, a write is not "set a key in the merged result". It is:

1. validate that the target layer is a valid destination
2. mutate that layer's logical map
3. record a patch set for save
4. re-materialize the store
5. save only when requested

## Why writes target a layer

Jubako intentionally requires a layer target for mutation APIs such as `SetTo`.

That constraint preserves:

- explicit ownership of the write destination
- correct write-back to the physical store
- separation between effective value and storage location

Without it, a merged configuration tree would not contain enough information to safely answer "where should this change
be persisted?"

## Validation

Before mutating a layer, the store validates:

- the store has been loaded
- the target layer exists
- the layer is writable and not read-only
- the path is valid
- sensitivity rules are not violated

Sensitivity is treated as a guardrail on write destinations. It is not itself the external storage routing mechanism.

## Dirty state

Jubako tracks dirty state at two levels:

- the layer has unsaved logical changes
- the store has at least one dirty layer

This is important because saving is layer-scoped. The store does not need to rewrite everything when only one layer has
changed.

Snapshot-aware layers can also become dirty because their physical projection is stale, even if the logical value did
not change directly.

## Save model

On save, Jubako persists only layers that both:

- support saving
- have pending changes or projection work

For ordinary document-backed layers, this means applying the accumulated patch set to the loaded bytes and delegating
to the source for persistence.

For contextual layers, the store passes a broader save context so the layer can reason about:

- the current logical state
- the logical state after a patch set
- schema metadata

This is what enables coordinated layers to route one logical change into multiple physical writes.
