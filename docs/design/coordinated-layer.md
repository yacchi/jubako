# Coordinated Layer

## Problem

Some configuration does not live in exactly one physical store.

A typical example is credentials:

- visible document-side fields should stay in a document layer
- secret material should live in a secure external store
- the application still wants one logical configuration model

If this split is implemented directly in application code, branching tends to leak everywhere. Jubako's coordinated
layer model keeps that logic behind a logical layer boundary.

## Design idea

A coordinated layer still looks like one layer to the store, but internally it may reconstruct and persist state across
multiple collaborators.

This changes the abstraction from:

- one path goes to one storage backend

to:

- one logical read may hydrate from multiple physical stores
- one logical write may fan out into multiple physical writes

In practice, the helper is easiest to reason about when it treats one entry as
three separate things:

1. current physical state
2. current logical state
3. target physical state

The current physical state may include document-side fields plus an already attached
external payload referenced by `secret_ref`. The helper should reconstruct the
current logical state first, then project that logical state onto the target
storage layout.

## Required store support

To support this cleanly, the store exposes narrow, read-only context objects:

- `SchemaView` for path and field metadata
- `StabilizeContext` for provisional logical snapshots
- `SaveContext` for before/after logical views

The store keeps ownership of orchestration. The coordinated layer decides how to project logical state onto its backing
stores.

## Why stabilization matters here

Coordinated layers often need an explicit stabilization step because the logical configuration may need correction or
hydration before the final resolved state is trustworthy.

Examples:

- migrating plaintext credentials into a secure store
- hydrating `storage:"keyring"` fields from a secret reference
- hydrating existing external payload even when the new route now points back to local storage
- marking a subtree dirty because physical projection no longer matches policy

## Intended boundary

Jubako itself does not need to define the meaning of tags such as `storage:"keyring"`.

Instead, Jubako provides enough schema visibility for application-owned coordinators to interpret those tags and to
implement domain-specific behavior.

This keeps the core generic while still making externalized configuration practical.

## Route interpretation

For helpers such as `external-store.NewMap`, `RouteForEntry` should be read as
"where should this entry be projected now?" rather than "does this entry
currently have external backing?".

That distinction matters for migration in both directions:

- `local -> external`: plaintext document-side fields are the current physical state, external is the target
- `external -> local`: external payload referenced by `ExistingRef` is still part of the current physical state, even if the target route is now local

This is why `ExistingRef` requires a stable `Ref` even when `UseExternal == false`.
When `ExternalKey` is omitted, the helper uses `Ref` as the backend key.

## `Ref` and `ExternalKey`

`Ref` and `ExternalKey` are related, but they do not always mean the same thing.

| Field | Meaning | Typical destination | Stability expectation |
|-------|---------|---------------------|-----------------------|
| `Ref` | the logical reference that remains in the document-side representation | stored in the document layer such as `secret_ref` | should stay stable for the lifetime of the logical attachment |
| `ExternalKey` | an optional physical lookup key override used against the external backend | passed to keyring, secret manager, or another external store | if specified, should usually stay stable for the lifetime of the ref |

In other words:

- `Ref` is what the document-side representation remembers
- `ExternalKey` is an optional override for what the external backend is asked to read, write, or delete

The default rule is simple:

- if `ExternalKey` is empty, the helper uses `Ref`

For many applications, that default is exactly what you want.

For example, after migrating one credential entry to keyring:

- the document layer may keep `secret_ref: "alice@example-space"`
- the backend call may also use `"alice@example-space"` as the keyring key

Here:

- `alice@example-space` is the logical attachment identity
- and, by default, also the concrete backend key

Only some applications need a separate `ExternalKey`. Typical reasons include:

- adding a backend-specific prefix or namespace
- adapting to backend character or length restrictions
- keeping a short stable ref in the document while using a different physical key externally

When a separate `ExternalKey` is used:

- `Ref`: what should remain in the document-side representation so this logical entry can point to external state later?
- `ExternalKey`: what exact key should the helper use right now to talk to the backend?

When `ExistingRef` is present, `RouteForEntry` must keep returning the same
`Ref`. If `ExternalKey` is omitted, the helper will continue using that ref as
the backend key. If a separate `ExternalKey` is used, `RouteForEntry` must still
return the key that corresponds to the already attached external state, even if
the target route now becomes local.

## Route design guidelines

When possible:

- derive `Ref` from a stable logical identity
- leave `ExternalKey` empty so it defaults to `Ref`

If you do override `ExternalKey`, derive it from that stable ref rather than
from mutable routing inputs.

Using mutable inputs such as tenant IDs, domains, or space identifiers inside
an overridden `ExternalKey` can create route drift while the entry still remains external.
The current helper does not automatically reproject secrets just because the
physical external key changed while `UseExternal` stayed true.

For current consumers, the safest rule is:

- treat `ExternalKey` as stable for the lifetime of a ref

If a use case truly requires mutable external keys, that should be treated as a
future enhancement with explicit reprojection support.

## Partial failure semantics

Coordinated save currently provides best-effort coordination with explicit error
propagation.

It does not provide rollback across multiple physical operations such as:

- external `Set`
- external `Delete`
- document-layer `Save`

Callers should assume that a mid-flight failure may leave some physical updates
applied and others unapplied. If stronger transactional guarantees are needed,
they should be added as a separate design improvement.
