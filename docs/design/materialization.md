# Materialization

## Role

Materialization is the step that turns per-layer maps into the resolved typed configuration value.

This is more than a simple merge. Jubako treats materialization as the point where these concerns come together:

- layer ordering
- stabilization of snapshot-aware layers
- origin tracking
- path remapping
- decoding into `T`

## Pipeline

The current pipeline is:

1. order layers by priority
2. run stabilization passes when layers implement `SnapshotAwareLayer`
3. merge layer data into a single logical map
4. rebuild origin indexes from the settled layer data
5. apply schema-based path mappings
6. decode into `T`
7. publish the resolved value through `Cell[T]`

## Stabilization

Stabilization exists for layers whose logical data depends on a broader snapshot, not just their own local storage.

Typical examples:

- a coordinated layer that reconstructs logical values from file metadata plus an external secret store
- a layer whose physical projection depends on another resolved path

The store runs stabilization in passes until the layer set converges. It also tracks a fingerprint to detect
oscillation.

## Merge semantics

The current default merge rule is:

- maps merge recursively
- non-maps replace earlier values

This keeps the behavior predictable and easy to explain. More advanced per-field strategies can be added later without
changing the basic materialization model.

## Origin tracking

After stabilization, the store walks each layer's settled data and records:

- leaf origins
- container origins

This is what makes `GetAt`, `GetAllAt`, and `Walk` able to answer provenance questions after merge.

The important detail is that origins are rebuilt from layer data, not inferred from the already merged result.

## Path remapping and decode

Before decoding into `T`, Jubako applies schema mappings derived from `jubako` tags and type conversion rules.

This separates two concerns:

- merge operates on the logical map space
- decode operates on the type-oriented schema space

That separation keeps the merge engine generic while still supporting schema-driven access patterns.
