# Runtime and Watch

## Resolved state

Jubako exposes the effective typed configuration through `Store[T]`, but the runtime model is intentionally split:

- `resolved *Cell[T]` holds the current typed value
- layer entries hold per-layer data and dirty state
- origin indexes hold provenance lookup structures

This prevents a reload from forcing the application to reconstruct the whole store object.

## Why `Cell[T]` exists

`Cell[T]` provides:

- lock-free reads of the current resolved value
- stable ownership of the runtime container
- subscription hooks for resolved updates

The key idea is that the container stays stable while the contained value changes.

This is why Jubako can support hot reload without forcing the rest of the application to rebuild its store wiring.

## Subscription model

Subscriptions are attached to the resolved runtime state, not to individual sources.

That means a subscriber sees the post-materialization result:

- after all relevant layers have been updated
- after stabilization has converged
- after path remapping and decode have completed

This keeps the subscription contract aligned with application-level configuration, not raw storage events.

## Watch model

`Store.Watch` asks each watchable layer to create a watcher, starts them, and fans their results into one debounced
reload loop.

Important properties:

- `WithNoWatch()` excludes a layer from store-level watching
- watcher results update layer data first
- the store then re-materializes
- subscribers and `OnReload` run only after a successful resolved update

## Why watching is store-managed

Store-managed watching gives Jubako a single place to enforce the runtime contract:

- multiple source changes can be batched
- one reload produces one coherent resolved update
- errors can be reported in a consistent way

Without a store-level watch loop, each layer would only know that "something changed", but not whether the total
configuration had reached a stable resolved state.
