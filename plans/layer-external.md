# External Layer Design Draft

## Problem

`backlog-cli` already supports `auth.credential_backend = auto|keyring|file`, but the interpretation and branching still live mostly in application code. The next step is to make jubako handle one logical configuration layer that can keep visible metadata in a document layer while routing secret fields to an external secure store such as keyring.

The target model is:

- the application still reads and writes one logical `Credential`
- the split between metadata storage and secret storage happens inside a jubako layer
- the schema, not ad-hoc application branching, defines which fields belong to which storage class

## What the current codebase already tells us

- `layer.Layer` is already the right seam for this feature. Store still treats each layer as one logical `Load(ctx) -> map[string]any` and `Save(ctx, changeset)` unit.
- `Store` currently keeps one `entry.data` and one `entry.changeset` per layer. That means save routing should happen inside the layer, not inside `Store`.
- `sensitive` is currently a validation/masking concern. It can prevent writing a sensitive field to a normal layer, but it does not choose the destination backend.
- `layer.StoreProvider` already exposes `SchemaType()`, `TagDelimiter()`, and `FieldTagName()`, which is enough to rebuild schema metadata if shared helpers exist.
- The current schema-building logic is usable, but it is still anchored in store internals (`buildMappingTable` in `decode.go`).

## Revised recommendation

### 1. Do not add `storage:secure` to jubako core tags yet

The latest concern is valid: `sensitive` and `storage:*` do not have the same kind of abstraction.

`sensitive` is a first-class jubako concern because jubako can give it concrete behavior:

- reject writes from sensitive fields into non-sensitive layers
- mask reads for origin/walk accessors
- preserve that semantic across all backends

By contrast, `storage:secure` would only name a routing class without jubako itself owning a stable, universal semantic for it. That makes it feel suspended between domain intent and implementation detail.

So the better current rule is:

- keep `sensitive` in jubako core
- keep storage routing out of jubako core tags unless jubako can define and enforce consistent cross-backend semantics for it

### 2. Put coordination semantics in a composite layer, not in `jubako` tags

The right seam is still a composite layer, but the required behavior is stronger than simple routing.

The `backlog-cli` example makes this clear:

- a logical secret value is written to keyring
- the file layer must also be updated with the lookup key or handle needed to read that value back later
- load needs both layers at once to reconstruct the single logical field

So the needed abstraction is not just:

- one logical path -> one child

It is:

- one logical write -> one or more child writes
- one logical read -> reconstruction from one or more children

This is a cleaner separation:

- jubako core owns merge, sensitivity, decoding, and layer composition primitives
- the composite layer owns cross-child coordination and split/fan-out mechanics
- the application or higher-level package owns the meaning of “this logical field externalizes into keyring + file metadata”

### 3. Give coordinators a read-only context, not direct Store mutation powers

The latest discussion makes coordinator requirements clearer:

- it may need the current logical config
- it may need the current child-layer states
- it may need schema/path metadata
- it may want to inspect application-specific tags to decide how projection works

That does justify more exposure than the initial `Classifier(path)` sketch, but it does not justify handing the coordinator a mutable Store reference.

The better shape is a read-only coordinator context.

Suggested responsibilities of that context:

- inspect the current logical config snapshot
- inspect current child-layer snapshots
- inspect a read-only schema/path view
- inspect raw or reflected field-tag metadata when needed

The coordinator should then return a plan. It should not directly mutate Store or child layers.

This keeps control flow explicit:

- jubako collects context
- the coordinator decides what logical state means and how writes expand
- the composite layer applies the returned child operations

### 4. Read-only schema access now looks justified, but only as a narrow view

The previous idea of `StoreProvider.Schema()` was too broad if it meant exposing the whole internal schema object.

But some read-only schema access now looks justified, because user-defined coordinators may need both:

- path mapping information
- access to raw field tags or reflected field metadata for their own directives

So the better direction is:

- extract the current schema-building logic into a shared helper
- expose a narrow read-only schema/path view to coordinators
- keep the internal mutable schema representation private

This read-only schema view should be designed around coordinator use cases, not around re-exporting internal store structures.

### 5. Build a coordinated composite layer, not only a routed layer

The new layer should still look like one logical layer to `Store`, while internally coordinating whatever collaborators the domain needs.

Responsibilities:

- `Load`: reconstruct one logical map from domain-owned physical stores
- `Stabilize`: inspect a provisional snapshot and declare dependencies / projection-dirty roots
- `Save`: apply one normalized patch stream, possibly writing to multiple physical stores for one logical field
- `Watch`: fan in child watchers when the underlying collaborators support it

The key difference is that a simple classifier is not expressive enough for all external-secret cases. `backlog-cli` needs coordinated persistence, not just destination selection.

One possible API direction after narrowing the design is:

```go
type SchemaView interface {
    Lookup(path string) (PathDescriptor, bool)
}

type PathDescriptor interface {
    Path() string
    FieldKey() string
    Sensitive() bool
    Tag(key string) (string, bool)
    StructField() reflect.StructField
}

type LoadContext interface {
    Schema() SchemaView
}

type StabilizeContext interface {
    Snapshot() map[string]any
    Schema() SchemaView
}

type SaveContext interface {
    Logical() map[string]any
    LogicalAfter(changes document.JSONPatchSet) (map[string]any, error)
    Schema() SchemaView
}

type StabilizeResult struct {
    Data            map[string]any
    Dependencies    []string
    Changed         bool
    ProjectionDirty []string
}

type Coordinator interface {
    Load(ctx context.Context, c LoadContext) (map[string]any, error)
    Stabilize(ctx context.Context, c StabilizeContext) (*StabilizeResult, error)
    Save(ctx context.Context, c SaveContext, changes document.JSONPatchSet) error
}

type SnapshotAwareLayer interface {
    layer.Layer
    Stabilize(ctx context.Context, c StabilizeContext) (*StabilizeResult, error)
}

func New(name layer.Name, coordinator Coordinator) layer.Layer
```

`New` can still return `layer.Layer` publicly. The concrete implementation would also satisfy an internal `SnapshotAwareLayer`-style interface so `Store` can run stabilization passes without changing ordinary layers.

The important boundary is:

- jubako does **not** need to interpret tags such as `storage:"keyring"` itself
- jubako only needs to expose them through `SchemaView`
- the coordinator can then decide what those tags mean for its own load/save logic

### 6. Keep MVP semantics narrow

The first version should deliberately stay conservative:

- one logical layer in origin/details APIs
- child-level origin visibility deferred
- child watchers merged best-effort; non-watchable children rely on explicit reload
- no new map-native backend abstraction yet

This keeps the public surface small while still acknowledging that Store-side stabilization is required internally.

### 7. Treat `sensitive` as orthogonal guardrails, not as the externalization mechanism

The current discussion clarifies that masking is not the core behavior needed for external stores. The key problem is coordinated persistence and reconstruction across multiple physical stores.

So:

- `sensitive` is still useful as a guardrail
- `sensitive` is not the modeling primitive for “write secret to keyring and write reference metadata to file”

### 8. Accept one important constraint from current sensitivity validation

`Store.Set*` validates sensitivity before the layer sees the save call. That means a routed/composite layer that owns any sensitive field must still be added as a sensitive layer from the Store point of view.

In practice, if a logical credentials layer can contain secret fields, the application should register that logical layer with `jubako.WithSensitive()`, even if some fields are persisted to a secure child and others to a normal document child.

This is an important MVP rule to document clearly.

## Concrete backlog-cli mapping model

With the coordinated-composite approach, backend selection becomes child coordination assembled from `credential_backend`:

- `credential_backend=file`
  - secret values and their metadata are both persisted through the file child
- `credential_backend=keyring`
  - secret values are persisted to the keyring child
  - the file child is also updated with the stable lookup key or handle required to read them back
  - load reconstructs one logical credential from file metadata + keyring value
- `credential_backend=auto`
  - resolve to one of the above at construction time

This means the design target is no longer plain routing. It is logical-to-physical projection across children, informed by read-only access to logical config, child state, and schema metadata.

## Concrete implementation sketch for backlog-cli credentials

The `feature/keyring-credentials` worktree already contains a concrete draft implementation, and that code is a better baseline than the earlier abstract sketch.

Observed model in the worktree:

```go
type ResolvedConfig struct {
    Credentials map[string]*Credential `json:"credential" jubako:"/credential"`
}

type ResolvedAuth struct {
    CredentialBackend CredentialSecretBackend `json:"credential_backend" jubako:"/auth/credential_backend,env:AUTH_CREDENTIAL_BACKEND"`
}

type Credential struct {
    AuthType      AuthType                `json:"auth_type,omitempty"`
    AccessToken   string                  `json:"access_token,omitempty" jubako:"sensitive"`
    RefreshToken  string                  `json:"refresh_token,omitempty" jubako:"sensitive"`
    ExpiresAt     time.Time               `json:"expires_at,omitempty"`
    APIKey        string                  `json:"api_key,omitempty" jubako:"sensitive"`
    UserID        string                  `json:"user_id,omitempty"`
    UserName      string                  `json:"user_name,omitempty"`
    SecretBackend CredentialSecretBackend `json:"secret_backend,omitempty"`
    SecretRef     string                  `json:"secret_ref,omitempty"`
}
```

There is also already a secret store abstraction:

```go
type credentialSecretStore interface {
    Get(ref string) (*credentialSecretPayload, error)
    Set(ref string, payload *credentialSecretPayload) error
    Delete(ref string) error
}
```

The draft keyring store uses one deterministic ref per profile:

```go
func credentialSecretRef(profileName string) string {
    return "profile/" + profileName
}
```

and stores one JSON payload per profile:

```go
type credentialSecretPayload struct {
    AccessToken  string `json:"access_token,omitempty"`
    RefreshToken string `json:"refresh_token,omitempty"`
    APIKey       string `json:"api_key,omitempty"`
}
```

So the current real design is:

- one logical `Credential` per profile
- one YAML metadata entry per profile
- zero or one keyring payload per profile

This is an important correction to the earlier abstract sketch: the current draft is **profile-level projection**, not per-secret-field projection.

For the target jubako design, this should be tightened further: backend selection belongs to parent auth config, not to each persisted `Credential`.

So the target shape should be closer to:

```go
type ResolvedAuth struct {
    CredentialBackend CredentialSecretBackend `json:"credential_backend" jubako:"/auth/credential_backend,env:AUTH_CREDENTIAL_BACKEND"`
}

type Credential struct {
    AuthType     AuthType  `json:"auth_type,omitempty"`
    AccessToken  string    `json:"access_token,omitempty" jubako:"sensitive"`
    RefreshToken string    `json:"refresh_token,omitempty" jubako:"sensitive"`
    ExpiresAt    time.Time `json:"expires_at,omitempty"`
    APIKey       string    `json:"api_key,omitempty" jubako:"sensitive"`
    UserID       string    `json:"user_id,omitempty"`
    UserName     string    `json:"user_name,omitempty"`
    SecretRef    string    `json:"secret_ref,omitempty"`
}
```

That is conceptually harder than the current worktree draft, but also cleaner:

- parent config decides where secrets should be stored
- each credential entry stores only the per-entry reference needed to rehydrate secrets
- legacy plaintext entries remain representable by simply omitting `SecretRef`

The coordinated design should preserve the current logical API:

```go
SetCredential(profileName string, cred *Credential) error
DeleteCredential(profileName string) error
Credential(profileName string) (*Credential, error)
```

### Physical children for `credential_backend=file`

The file backend is the easy case:

- one YAML child
- no external store
- merge is effectively identity
- plan writes only to the YAML child

This backend is the compatibility baseline and should be implementable with almost no special logic.

### Physical children for `credential_backend=keyring`

The keyring backend needs at least two physical children:

1. **metadata child**: traversable YAML file (`credentials.yaml`)
2. **secret child**: keyring adapter addressed by deterministic references

The YAML child stores non-secret credential fields plus reference metadata.

Possible YAML shape:

```yaml
credential:
  default:
    auth_type: oauth
    expires_at: 2026-04-24T00:00:00Z
    user_id: "100"
    user_name: "alice"
    secret_ref: profile/default
```

Possible keyring representation:

- service: `github.com/yacchi/backlog-cli`
- ref: `profile/default`
- value: JSON payload containing `access_token`, `refresh_token`, and `api_key`

The exact reference string can vary, but it should be deterministic so that:

- the same logical credential rewrites the same keyring entry
- orphan cleanup is possible
- metadata does not need random IDs just to find the secret again

### Important implication: the current draft is already a mixed model

The keyring library cannot traverse all entries for a subtree, so the worktree draft does not try to make keyring a fully traversable configuration layer.

Instead, it already uses two different child kinds:

1. **snapshot child**: a normal `layer.Layer` that can load/save a map (YAML file)
2. **lookup child**: a store that can get/set/delete values by explicit reference (keyring)

That makes the draft much more concrete: a future jubako design should probably support this mixed model instead of insisting that every child is a `layer.Layer`.

### Load flow for `credential_backend=keyring`

Concrete loading flow:

1. load `credentials.yaml`
2. iterate `/credential/*`
3. for each profile, inspect `secret_ref`
4. if `secret_ref` is present, ask the keyring store for that single ref
5. merge the payload back into the in-memory `Credential`
6. return one logical map under `/credential`

This is why a subtree-level credential projector is a better fit than leaf-level routing: the projector wants to reconstruct one `Credential` object per profile entry.

This also avoids a load-time dependency on parent config. Load only needs persisted per-entry state plus keyring lookups, so it does not create a circular dependency on the final resolved snapshot.

### Save flow for `SetCredential(profileName, cred)`

Concrete planning flow:

1. derive deterministic secret refs for `profileName`
2. split `cred` into:
   - metadata fields: `auth_type`, `expires_at`, `user_id`, `user_name`
   - secret fields: `access_token`, `refresh_token`, `api_key`
3. build one keyring payload from the secret fields
4. build YAML patch operations for metadata fields plus `secret_ref`
5. apply them in a safety-oriented order

Recommended write order:

1. write/update keyring entries first
2. save YAML metadata second

Reason:

- if keyring write fails, YAML is unchanged, so no broken references are introduced
- if YAML save fails after keyring succeeded, orphaned secrets are possible, but logical config remains readable from the previous metadata state

This is not perfect transactional behavior, but it is safer than exposing references to missing secrets.

Observed current draft behavior:

- for `keyring`, `SetCredential` stores the payload first, then writes YAML metadata
- for `auto`, it tries keyring first and falls back to file if keyring write fails
- `resolveCredential` in the current worktree uses `secret_backend=keyring`; the target design should instead treat `secret_ref != \"\"` as the hydration trigger
- legacy plaintext entries without `secret_ref` are still returned as-is

### Important correction: `JSONPatchSet` is not sufficient for coordinated save

This is the key difficulty for the backlog-cli use case.

If the user changes:

```yaml
auth:
  credential_backend: file -> keyring
```

then the logical value of `/credential/{profile}` may remain unchanged:

- `Credential()` returns the same token values before and after
- the effective config looks identical to readers

But the physical projection must still change:

- secrets must move from `credentials.yaml` to keyring
- `secret_ref` must be added to credential metadata
- plaintext secret fields must disappear from file storage

That means a save model based only on the credentials layer's own `JSONPatchSet` is insufficient.

With the current jubako save flow:

- changing `/auth/credential_backend` dirties the auth/user layer
- the credentials layer may have no logical patch at all
- `Store.Save()` only saves dirty layers
- `saveLayerLocked()` is a no-op when the layer has no changeset

So under the current model, a file -> keyring migration has no reliable way to trigger credentials-layer persistence work.

### Therefore the first-class problem is not “apply patch”, but “reconcile projection”

For coordinated layers, save should be modeled as:

1. determine the desired physical representation from the resolved config
2. compare it with the layer's current physical state
3. perform the physical operations needed to reconcile the two

This is fundamentally different from:

1. receive a local `JSONPatchSet`
2. forward that patch to one document backend

### What backlog-cli needs from jubako save semantics

For the credentials layer, the save decision depends on:

1. **before logical snapshot**
2. **after logical snapshot**
3. **current physical credential metadata state**
4. **current parent auth config**, especially `/auth/credential_backend`

The save entry point therefore needs richer inputs than local layer patches. At the same time, the coordinator does not need a separate public “projection dirty” channel if jubako normalizes those dirty roots into synthetic subtree patches before save.

The first useful shape is something like:

```go
type SaveContext interface {
    Logical() map[string]any
    LogicalAfter(changes document.JSONPatchSet) (map[string]any, error)
    Schema() SchemaView
}

type Coordinator interface {
    Load(ctx context.Context, c LoadContext) (map[string]any, error)
    Stabilize(ctx context.Context, c StabilizeContext) (*StabilizeResult, error)
    Save(ctx context.Context, c SaveContext, changes document.JSONPatchSet) error
}
```

In this model:

- `Load()` still returns the logical map for the layer
- `Stabilize()` discovers dependencies and projection-dirty roots from a provisional snapshot
- `Save()` receives one normalized patch stream containing both real edits and synthetic reprojection edits

### Coordinated layers need save dependencies

A coordinated layer also needs a way to declare which resolved paths can affect its physical projection.

For backlog-cli credentials, relevant dependencies include at least:

- `/credential`
- `/auth/credential_backend`

When either changes, the coordinated credentials layer should be considered “reconcile-dirty” even if its own local logical patchset is empty.

This is what enables:

- file -> keyring migration
- keyring -> file migration
- auto mode re-projection when backend resolution changes

### backlog-cli-specific implication

For backlog-cli, the credentials layer is not just “the owner of `/credential`”.
It is really a projection layer whose output depends on both:

- the credential subtree
- auth-level backend policy

That means the abstraction should explicitly support **cross-path save dependencies**, rather than pretending everything can be derived from the layer's own local patch set.

### New preferred direction: keep save mostly as-is, and make load snapshot-aware

The discussion suggests a better center of gravity:

- do **not** make save semantics dramatically richer first
- instead, let special layers participate in a snapshot-aware load stabilization phase

For backlog-cli credentials, this is a much better fit:

- the credentials layer can read its own metadata source and keyring source directly
- after a provisional global snapshot exists, it can inspect parent config such as `/auth/credential_backend`
- if it detects “logical value is fine, but physical projection should migrate”, it can mark itself as needing persistence

This preserves the user-facing mental model:

- `Load()` resolves the effective config
- `Save()` persists dirty layers

but it gives load enough power to discover migration work.

### Refined loading model: base load + stabilization passes

The safer formulation is not “give every layer a half-built snapshot during raw load”.
Instead:

1. **Base load phase**
   - every layer performs its ordinary source reads
   - no snapshot-dependent logic yet
2. **Provisional materialization**
   - jubako builds a temporary resolved config snapshot from the current layer outputs
3. **Stabilization phase**
   - snapshot-aware layers receive the provisional snapshot
   - they may refine their logical output
   - they may declare dependencies
   - they may mark themselves as pending-save/dirty
4. **Repeat until fixed point**
    - if any layer output changed, materialize again
    - rerun only affected snapshot-aware layers if dependency tracking is available
    - stop when no layer changes, or fail on oscillation / pass limit

This avoids exposing partially-built state while still enabling parent-dependent refinement.

This still fits the mental model of “one logical layer” from the Store's point of view. The important caveat is that this cannot be implemented by the current plain `layer.Layer` contract alone; it needs Store-level orchestration for the stabilization passes. So it remains a layer as a public abstraction, but requires an optional extended lifecycle behind the scenes.

### Loop prevention is mandatory

A stabilization loop needs hard safety rails.

Minimum requirements:

1. a global max-pass limit
2. per-pass fingerprints of layer output + dependency declarations + projection-dirty state
3. oscillation detection when the same fingerprint sequence repeats

If possible, jubako should also report which layers participated in the loop. That is much more actionable than a generic “did not stabilize” error.

### Distinguish two concepts: unstable vs dirty

The discussion surfaced two different notions that should not be conflated:

1. **unstable / needs another pass**
   - the layer's resolved logical output may change if re-evaluated against a newer snapshot
2. **dirty / pending save**
   - the layer's physical sources should be rewritten on the next save

For backlog-cli migration:

- changing `/auth/credential_backend` from `file` to `keyring` may leave the logical credential unchanged
- therefore the layer may be **stable** logically
- but still become **dirty for save**, because plaintext file storage should migrate to keyring

That distinction is essential.

### Refine dirty tracking: use path/subtree-level projection-dirty state

The next refinement is to avoid thinking of dirty only at whole-layer granularity.

For coordinated layers, the more useful unit is usually:

- one field path, or
- one subtree root

For backlog-cli credentials, good dirty units are:

- `/credential`
- `/credential/default`
- `/credential/{profile}`

This enables the exact behavior we want:

1. load legacy plaintext credentials from file
2. build the logical `Credential` normally
3. inspect parent config `/auth/credential_backend`
4. if backend says `keyring` but the profile is still plaintext-in-file:
   - keep the logical value unchanged
   - mark `/credential/{profile}` as **projection-dirty**
5. later, `Save()` sees that projection-dirty subtree and routes it through the normal keyring/file write path

So yes: if jubako can carry dirty information per path/subtree, backlog-cli can avoid “migrate during load” and instead do **migrate on next save through the regular projection path**.

### Save can stay patch-oriented if projection-dirty paths are surfaced as synthetic subtree patches

This suggests a simpler bridge back to existing save semantics:

- normal user edits still produce ordinary `JSONPatchSet`
- stabilization may add **projection-dirty paths**
- before save, jubako can normalize those paths into synthetic subtree patches

That does not have to mean “invent fake user-visible value changes”. It only needs to mean:

- this subtree must be included when the coordinated layer computes its outgoing writes
- if the subtree currently exists, emit a synthetic `replace` at the subtree root
- if it does not exist, emit a synthetic `add`

For backlog-cli, that means:

- even if `/credential/default` has the same logical value before and after
- if it is marked projection-dirty, `Save()` still includes that subtree when calling the credentials coordinator

### This keeps migration on the normal save path

That gives the desired behavior:

- `Load()` never silently persists anything
- the application can use the config immediately
- the next `Save()` performs the migration
- the migration still goes through the same routing/projection logic as ordinary updates

Conceptually:

```go
type StabilizeResult struct {
    Data            map[string]any
    Dependencies    []string
    Changed         bool
    ProjectionDirty []string // JSONPointer paths or subtree roots
}
```

That means `ProjectionDirtyPaths()` may only be an internal normalization step, not a public API requirement.

The public save surface can stay closer to today's model if jubako rewrites projection-dirty subtrees into synthetic patches before the coordinator sees them.

Conceptually:

```go
func ExpandProjectionDirtyPatches(
    base document.JSONPatchSet,
    dirty []string,
    current map[string]any,
) document.JSONPatchSet
```

Then the coordinated layer's save input can remain:

```go
type SaveContext interface {
    Logical() map[string]any
    LogicalAfter(changes document.JSONPatchSet) (map[string]any, error)
    Schema() SchemaView
}
```

The coordinator then only sees one stream of subtree patches:

1. real user edits
2. synthetic reprojection edits

This is likely the cleanest path for backlog-cli.

### Interface direction for fixed-point load

The coordinator/layer likely needs a second lifecycle hook after base load:

```go
type LoadContext interface {
    Schema() SchemaView
}

type StabilizeContext interface {
    Snapshot() map[string]any
    Schema() SchemaView
}

type StabilizeResult struct {
    Data         map[string]any
    Dependencies []string
    Changed      bool
    ProjectionDirty []string
}

type Coordinator interface {
    Load(ctx context.Context, c LoadContext) (map[string]any, error)
    Stabilize(ctx context.Context, c StabilizeContext) (*StabilizeResult, error)
    Save(ctx context.Context, c SaveContext, changes document.JSONPatchSet) error
}
```

The most important part is not the exact names, but the lifecycle split:

- `Load` does direct source reads
- `Stabilize` can inspect a provisional global snapshot
- `Save` can remain patch-oriented because projection-dirty roots are normalized into synthetic subtree patches before the coordinator sees them

### backlog-cli migration under this model

For `credential_backend: file -> keyring`, the credentials layer could do this:

1. base-load metadata from `credentials.yaml`
2. hydrate any existing `secret_ref` entries from keyring
3. provisional snapshot is built
4. `Stabilize` sees `/auth/credential_backend == keyring`
5. for any credential still stored as plaintext in metadata:
   - keep the logical `Credential` value unchanged
   - mark the layer `Dirty`
   - prepare internal pending migration state:
     - target keyring payload
     - target metadata rewrite with `secret_ref`
6. since logical output did not change, no extra pass may be needed
7. later, normal `Save()` persists that prepared migration

This is a much better fit for the backlog-cli use case than trying to express migration only through local credential-layer patches.

### Delete flow for `DeleteCredential(profileName)`

Concrete delete flow:

1. read the current metadata entry for that profile
2. inspect `secret_ref`
3. delete the YAML subtree for `/credential/{profile}`
4. delete the referenced keyring payload when applicable

The current worktree draft actually deletes the keyring payload before removing the YAML entry. That works, but it is exactly the kind of ordering choice the jubako-level design should make explicit.

### The first implementation target may be narrower than a fully generic composite layer

The concrete backlog-cli requirement suggests a pragmatic first scope:

- one coordinated credentials layer
- one traversable YAML metadata child
- one non-traversable keyring lookup child
- one credential projector applied to `map[string]Credential`

If this works well, jubako can later generalize it into a broader coordinated-composite framework. But the first implementation does not need to solve every possible multi-child projection shape at once.

## If backlog-cli were implemented on top of the proposed jubako interfaces

This section turns the design into a concrete “how would backlog-cli actually use it?” proposal.

### Proposed jubako-side interfaces for the first usable version

The previous `SnapshotChild` / `LookupChild` proposal was probably too generic for the first usable version.

For the backlog-cli case, the more natural shape is:

- jubako provides a thin `coordinated.New(name, coordinator)` wrapper
- the user-implemented coordinator directly owns whatever collaborators it needs
- for backlog-cli, that means the coordinator directly holds:
  - one metadata `layer.Layer`
  - one `credentialSecretStore`

That keeps the abstraction honest: jubako coordinates lifecycle, but domain-specific storage topology stays in the coordinator implementation.

The first practical interface can therefore be much smaller:

```go
package coordinated

type LoadContext interface {
    Schema() SchemaView
}

type StabilizeContext interface {
    Snapshot() map[string]any
    Schema() SchemaView
}

type SaveContext interface {
    Logical() map[string]any
    LogicalAfter(changes document.JSONPatchSet) (map[string]any, error)
    Schema() SchemaView
}

type StabilizeResult struct {
    Data            map[string]any
    Dependencies    []string
    Changed         bool
    ProjectionDirty []string
}

type SchemaView interface {
    Lookup(path string) (PathDescriptor, bool)
}

type Coordinator interface {
    Load(ctx context.Context, c LoadContext) (map[string]any, error)
    Stabilize(ctx context.Context, c StabilizeContext) (*StabilizeResult, error)
    Save(ctx context.Context, c SaveContext, changes document.JSONPatchSet) error
}

type SnapshotAwareLayer interface {
    layer.Layer
    Stabilize(ctx context.Context, c StabilizeContext) (*StabilizeResult, error)
}

func New(name layer.Name, coordinator Coordinator) layer.Layer
```

Optional extension points can be added later if needed:

- `type WatchCoordinator interface { Watch(opts ...layer.WatchOption) (layer.LayerWatcher, error) }`
- `type DetailsCoordinator interface { FillDetails(*types.Details) }`

This is intentionally much narrower. It is just enough to express:

- read-only access to the resolved logical config for save-time decisions
- coordinator-owned domain-specific collaborators
- projection-dirty subtree reporting during stabilization
- custom save ordering implemented by the coordinator itself
- a public return type that still looks like `layer.Layer`

### Important correction: user coordinators should not do raw map decoding by hand

The previous sketch was still too low-level. If the coordinator has to manually implement `decodeCredential(raw)`-style logic, jubako is leaking work that should stay inside its existing schema/decoder pipeline.

The first usable design should let coordinators reuse:

- struct definitions
- jubako path/schema mapping
- the configured decoder
- struct expansion for writes

So the public surface likely needs **helper functions**, not larger coordinator interfaces.

Possible helper direction:

```go
type DecodeContext interface {
    Schema() SchemaView
}

func DecodeAt[T any](c DecodeContext, raw map[string]any, path string) (T, error)
func BeforeAt[T any](c SaveContext, path string) (T, bool, error)
func AfterAt[T any](c SaveContext, changes document.JSONPatchSet, path string) (T, bool, error)
func BuildStructPatch(path string, v any, opts ...jubako.SetOption) (document.JSONPatchSet, error)
func TaggedDescendants(
    schema SchemaView,
    root string,
    tagKey string,
    tagValue string,
) ([]PathDescriptor, error)
```

These helpers would let a coordinator stay at the same abstraction level as normal jubako users: working with typed structs and standard tag-based mapping, instead of hand-walking `map[string]any`.

### Example: user-defined `storage:"keyring"` tags

This is the missing bridge to implementation: the application can place its own tag directives on the structs it already passes to jubako.

For example:

```go
type AuthConfig struct {
    CredentialBackend CredentialBackend `json:"credential_backend"`
}

type Credential struct {
    BaseURL      string `json:"base_url"`
    AccessToken  string `json:"access_token" storage:"keyring"`
    RefreshToken string `json:"refresh_token" storage:"keyring"`
    APIKey       string `json:"api_key" storage:"keyring"`
    SecretRef    string `json:"secret_ref,omitempty"`
}

type Config struct {
    Auth       AuthConfig              `json:"auth"`
    Credential map[string]*Credential  `json:"credential"`
}
```

The meaning of `storage:"keyring"` is still completely application-owned. jubako only needs to preserve enough schema information so a coordinator can ask:

- which fields under `/credential/{profile}` carry `storage:"keyring"`?
- which fields are ordinary metadata and should stay in the YAML document?

That keeps `storage` out of jubako core semantics while still making it practical to implement coordinated external storage.

### How load would use those tags

On load, backlog-cli can decode the ordinary metadata struct first, then hydrate only the tagged fields from keyring:

```go
func (c *credentialCoordinator) hydrateTaggedSecretFields(
    cc coordinated.LoadContext,
    profilePath string,
    credMap map[string]any,
    payload map[string]any,
) error {
    fields, err := coordinated.TaggedDescendants(
        cc.Schema(),
        profilePath,
        "storage",
        "keyring",
    )
    if err != nil {
        return err
    }
    for _, field := range fields {
        rel := strings.TrimPrefix(field.Path(), profilePath)
        v, ok := jsonptr.GetPath(payload, rel)
        if !ok {
            continue
        }
        if result := jsonptr.SetPath(credMap, rel, v); !result.Success {
            return fmt.Errorf("set %q: %s", rel, result.Reason)
        }
    }
    return nil
}
```

So the load flow becomes:

1. decode metadata into the logical credential shape
2. inspect `secret_ref`
3. load the keyring payload when needed
4. hydrate only the fields tagged `storage:"keyring"`

This is exactly where user-defined tags become useful: the coordinator does not need a hard-coded list of secret fields.

### How save would use those tags

On save, the same tags let the coordinator split one logical credential into:

- the secret payload written to keyring
- the metadata payload written to file

Sketch:

```go
func (c *credentialCoordinator) splitTaggedSecretFields(
    cc coordinated.SaveContext,
    profilePath string,
    credMap map[string]any,
) (metadata map[string]any, secret map[string]any, err error) {
    metadata = container.DeepCopyMap(credMap)
    secret = make(map[string]any)

    fields, err := coordinated.TaggedDescendants(
        cc.Schema(),
        profilePath,
        "storage",
        "keyring",
    )
    if err != nil {
        return nil, nil, err
    }
    for _, field := range fields {
        rel := strings.TrimPrefix(field.Path(), profilePath)
        v, ok := jsonptr.GetPath(metadata, rel)
        if !ok {
            continue
        }
        if result := jsonptr.SetPath(secret, rel, v); !result.Success {
            return nil, nil, fmt.Errorf("set secret %q: %s", rel, result.Reason)
        }
        if deleted := jsonptr.DeletePath(metadata, rel); !deleted {
            return nil, nil, fmt.Errorf("delete metadata %q", rel)
        }
    }
    return metadata, secret, nil
}
```

And stabilization can use the same schema helper to detect whether the metadata subtree still contains tagged secret fields:

```go
func containsTaggedSecretFields(
    schema coordinated.SchemaView,
    profilePath string,
    credMap map[string]any,
) (bool, error) {
    fields, err := coordinated.TaggedDescendants(schema, profilePath, "storage", "keyring")
    if err != nil {
        return false, err
    }
    for _, field := range fields {
        rel := strings.TrimPrefix(field.Path(), profilePath)
        if _, ok := jsonptr.GetPath(credMap, rel); ok {
            return true, nil
        }
    }
    return false, nil
}
```

That makes the save behavior straightforward:

1. decode the logical credential after applying patches
2. split the subtree by tag
3. write the `storage:"keyring"` portion to keyring
4. write the remaining metadata portion to the YAML layer
5. inject `secret_ref` into metadata as part of the coordinator's normal projection

This is the clearest implementation story so far:

- users annotate their own structs
- jubako exposes those tags through schema helpers
- coordinators use the tags during load/save
- jubako itself stays neutral about the meaning of `storage`

### Important correction: load-time and save-time context should be asymmetric

The cycle concern is real:

- `Load()` cannot depend on a fully materialized final config if that config itself depends on this layer's load result
- `Stabilize()` can depend on a provisional snapshot
- `Save()` can depend on the logical before/after config because saving happens after stabilization/materialization

So the first design should embrace this asymmetry:

- `LoadContext` should be intentionally narrow and avoid promising a full resolved snapshot
- `StabilizeContext` can provide a provisional snapshot
- `SaveContext` can provide `Logical()` and `LogicalAfter(changes)`
- projection-dirty remains an internal Store concern until it is expanded into synthetic subtree patches

For backlog-cli credentials, that is acceptable because:

- load-time hydration can rely on stored metadata (`secret_ref`) plus keyring lookups
- save-time routing can consult the resolved config, including `/auth/credential_backend`

### backlog-cli wiring proposal

If backlog-cli used that interface, the credentials layer setup could look like this:

```go
func newCredentialsLayer(credentialsPath string) layer.Layer {
    return coordinated.New(
        LayerCredentials,
        newCredentialCoordinator(credentialsPath),
    )
}
```

And `newConfigStore()` would add that coordinated layer instead of a plain YAML layer:

```go
if err := store.Add(
    newCredentialsLayer(credentialsPath),
    jubako.WithSensitive(),
    jubako.WithOptional(),
); err != nil {
    return nil, err
}
```

### backlog-cli coordinator sketch

The coordinator would absorb the logic that currently lives in `config/store.go` and `credential_secret_store.go`.

```go
type credentialCoordinator struct {
    metadata    layer.Layer
    secretStore credentialSecretStore
}

func newCredentialCoordinator(credentialsPath string) *credentialCoordinator {
    return &credentialCoordinator{
        metadata: layer.New(
            "credentials-meta",
            fs.New(credentialsPath, fs.WithFileMode(0600)),
            yaml.New(),
        ),
        secretStore: newCredentialSecretStore(),
    }
}
```

#### Load

On load, the coordinator would:

1. load metadata from `c.metadata`
2. inspect `/credential/{profile}`
3. if `secret_ref` is empty, return the entry as-is
4. otherwise read `secret_ref` from `c.secretStore`
5. decode the payload and hydrate only fields tagged `storage:"keyring"`
6. return the resolved logical config map

Sketch:

```go
func (c *credentialCoordinator) Load(
    ctx context.Context,
    cc coordinated.LoadContext,
) (map[string]any, error) {
    metadata, err := c.metadata.Load(ctx)
    if err != nil {
        return nil, err
    }
    out := container.DeepCopyMap(metadata)

    creds, _ := jsonptr.GetPath(out, "/credential")
    for profileName, raw := range credentialEntries(creds) {
        cred, err := coordinated.DecodeAt[*Credential](cc, map[string]any{"value": raw}, "/value")
        if err != nil {
            return nil, err
        }
        if cred.SecretRef == "" {
            continue
        }

        payload, err := c.secretStore.Get(cred.SecretRef)
        if err != nil {
            return nil, err
        }
        credMap, ok := raw.(map[string]any)
        if !ok {
            return nil, fmt.Errorf("credential %q is %T, want map[string]any", profileName, raw)
        }
        if err := c.hydrateTaggedSecretFields(
            cc,
            PathCredential+"/"+jsonptr.Escape(profileName),
            credMap,
            payload,
        ); err != nil {
            return nil, err
        }
    }
    return out, nil
}
```

#### Stabilize

On stabilize, the coordinator can inspect the provisional snapshot and flag migration candidates without changing logical values.

For backlog-cli, the key case is:

1. `/auth/credential_backend == keyring`
2. `/credential/{profile}` still contains plaintext secret fields in metadata
3. `secret_ref` is empty

Then the coordinator should:

- leave the logical `Credential` unchanged
- return `ProjectionDirty: []string{"/credential/{profile}"}` for that subtree

Sketch:

```go
func (c *credentialCoordinator) Stabilize(
    ctx context.Context,
    cc coordinated.StabilizeContext,
) (*coordinated.StabilizeResult, error) {
    snapshot := cc.Snapshot()
    auth, err := coordinated.DecodeAt[ResolvedAuth](cc, snapshot, "/auth")
    if err != nil {
        return nil, err
    }
    if normalizeCredentialSecretBackend(auth.CredentialBackend) != CredentialSecretBackendKeyring {
        return &coordinated.StabilizeResult{}, nil
    }

    dirty := make([]string, 0)
    creds, _ := jsonptr.GetPath(snapshot, "/credential")
    for profileName, raw := range credentialEntries(creds) {
        cred, err := coordinated.DecodeAt[*Credential](cc, map[string]any{"value": raw}, "/value")
        if err != nil {
            return nil, err
        }
        profilePath := PathCredential + "/" + jsonptr.Escape(profileName)
        credMap, ok := raw.(map[string]any)
        if !ok {
            return nil, fmt.Errorf("credential %q is %T, want map[string]any", profileName, raw)
        }
        tagged, err := containsTaggedSecretFields(cc.Schema(), profilePath, credMap)
        if err != nil {
            return nil, err
        }
        if cred.SecretRef == "" && tagged {
            dirty = append(dirty, profilePath)
        }
    }
    return &coordinated.StabilizeResult{
        ProjectionDirty: dirty,
    }, nil
}
```

#### Save

By the time save reaches the coordinator, ordinary edits and projection-dirty roots have already been normalized into one patch stream.

For each affected profile:

1. read `before` from `cc.Logical()`
2. read `after` from `cc.LogicalAfter(changes)`
3. inspect `after.Auth.CredentialBackend`
4. derive affected profiles from the normalized patches
5. execute metadata/keyring operations directly according to the backend and operation kind

Sketch:

```go
func (c *credentialCoordinator) Save(
    ctx context.Context,
    cc coordinated.SaveContext,
    changes document.JSONPatchSet,
) error {
    // `changes` already includes synthetic subtree patches expanded from
    // projection-dirty roots during Store-level save orchestration.
    profiles := changedCredentialProfiles(changes)
    for _, profileName := range profiles {
        profilePath := PathCredential + "/" + jsonptr.Escape(profileName)

        before, _, err := coordinated.BeforeAt[*Credential](cc, profilePath)
        if err != nil {
            return err
        }
        after, _, err := coordinated.AfterAt[*Credential](cc, changes, profilePath)
        if err != nil {
            return err
        }
        auth, _, err := coordinated.AfterAt[ResolvedAuth](cc, changes, "/auth")
        if err != nil {
            return err
        }
        backend := normalizeCredentialSecretBackend(auth.CredentialBackend)

        if err := c.saveProfile(ctx, cc, profilePath, profileName, before, after, backend); err != nil {
            return err
        }
    }
    return c.saveMetadata(ctx, cc, changes)
}
```

The coordinator can still use an internal plan/step model if it wants, but that planning object does not need to be pushed into jubako's public API on day one.

### How `saveProfile` would behave in backlog-cli

#### 1. `backend=file`

- write full `Credential` subtree into the metadata map
- clear `secret_ref`
- if `before` referenced keyring, delete the old keyring payload

#### 2. `backend=keyring`

- derive ref from `before.SecretRef` or `credentialSecretRef(profileName)`
- split the logical credential subtree with `splitTaggedSecretFields(...)`
- encode one keyring payload from only the fields tagged `storage:"keyring"`
- write the payload via `c.secretStore.Set`
- persist the remaining metadata subtree after tagged secret fields are removed
- set `secret_ref: <ref>`

Sketch:

```go
func (c *credentialCoordinator) persistKeyringProfile(
    ctx context.Context,
    cc coordinated.SaveContext,
    profilePath string,
    profileName string,
    credMap map[string]any,
    before *Credential,
) error {
    metadata, secret, err := c.splitTaggedSecretFields(cc, profilePath, credMap)
    if err != nil {
        return err
    }

    ref := credentialSecretRef(profileName)
    if before != nil && before.SecretRef != "" {
        ref = before.SecretRef
    }
    if err := c.secretStore.Set(ref, secret); err != nil {
        return err
    }
    if result := jsonptr.SetPath(metadata, "/secret_ref", ref); !result.Success {
        return fmt.Errorf("set secret_ref: %s", result.Reason)
    }

    return c.writeProfileMetadata(ctx, profilePath, metadata)
}
```

#### 3. `backend=auto`

The simplest backlog-cli-owned implementation can stay close to the current worktree code:

- try `c.secretStore.Set`
- if it succeeds, persist metadata as `keyring`
- if it fails, fall back to `file`

This fallback logic can remain entirely in the coordinator.

Projection-dirty paths are especially important here:

- if backend changed from `file` to `keyring`
- and `/credential/default` is unchanged logically
- stabilization can still mark `/credential/default` as projection-dirty
- Store expands that root into a synthetic subtree patch
- then `Save()` includes it in the same `saveProfile` flow as ordinary edits

#### 4. delete

If `after == nil`:

- remove `/credential/{profile}` from metadata
- if `before.SecretRef != ""`, delete the keyring payload too

### Metadata persistence in backlog-cli

The coordinator directly owns the metadata layer, so it can also decide how to save metadata changes:

```go
func (c *credentialCoordinator) saveMetadata(
    ctx context.Context,
    cc coordinated.SaveContext,
    changes document.JSONPatchSet,
) error {
    patches := coordinated.FilterPatches(changes, func(path string) bool {
        return strings.HasPrefix(path, "/credential/")
    })
    return c.metadata.Save(ctx, patches)
}
```

In practice, backlog-cli may need a slightly smarter helper than `FilterPatches`, because metadata writes need secrets removed and `secret_ref` injected. The point is that those transformations should still be built from jubako's struct/schema helpers, not from handwritten map decoding.

### Resulting backlog-cli Store API becomes thin again

If the coordinated layer handles load and save, the backlog-cli store methods can go back to thin wrappers:

```go
func (s *Store) SetCredential(profileName string, cred *Credential) error {
    return s.store.Set(
        LayerCredentials,
        jubako.Struct(PathCredential+"/"+profileName, cred),
        jubako.SkipZeroValues(),
    )
}

func (s *Store) DeleteCredential(profileName string) error {
    return s.store.DeleteFrom(LayerCredentials, PathCredential+"/"+profileName)
}

func (s *Store) Credential(profileName string) (*Credential, error) {
    resolved := s.store.Get()
    return resolved.GetCredential(profileName), nil
}
```

That is still a good test for the abstraction: if `backlog-cli` keeps large custom branches in `Store.SetCredential` and `Store.Credential`, jubako has not absorbed enough of the coordination model yet.

### What should remain optional, not first-class, in jubako

The following may still be useful later, but do not need to be first-class in the initial API:

- generic child registries such as `SnapshotChild` / `LookupChild`
- generic public `Plan` / `Step` / `LookupOp` types
- generic lookup-store abstractions beyond what backlog-cli itself needs

They can be added later if a second or third real use case needs the same shape.
### Minimum backlog-cli-specific code that should remain outside jubako

Even with the proposed interface, these pieces should probably stay in backlog-cli:

- `CredentialSecretBackend` enum (`auto|keyring|file`)
- deterministic ref convention (`profile/<name>`)
- credential payload JSON schema
- the credential-specific coordinator implementation

That keeps jubako generic while still letting backlog-cli use it concretely.

## Structural projection is a better fit than pinpoint path routing

The `keyring` use case suggests that pinpoint path routing is the wrong default abstraction.

Why:

- dynamic maps such as `map[string]Credential` would require enumerating unstable keys
- the real intent is usually “for each logical credential entry, externalize these parts”
- projection logic often wants to operate on one subtree at a time, not on isolated leaf paths

So the better model is structural projection:

- identify a subtree shape or domain object
- apply one coordinator/projector to every matching subtree
- let that projector reconstruct logical values and expand writes across children

For example:

- `map[string]Credential`
- apply the same credential projector to each map value
- when backend is keyring, write secrets to keyring and write lookup metadata to file
- when backend is file, persist the whole logical credential to file

This is much easier to use than forcing applications to enumerate every effective path.

## What should identify a projection target?

The best answer is probably not “exact path only”.

Better candidates:

1. Go type of the subtree value
2. Container field whose elements/values should use a projector
3. Schema descriptor that can match struct fields, slice elements, or map values

This suggests a layered approach:

- the composite layer traverses logical/schema structure
- it finds matching subtrees structurally
- it invokes the projector/coordinator for each subtree instance

If tags are used at all, they should likely mark the subtree/container level, not each leaf field.

## Load/merge semantics to prefer

The coordinated layer should define an explicit child merge order and reconstruction step.

Recommended default:

1. load the document child first
2. load external children
3. let the coordinator reconstruct logical values from the combined child state

This gives good migration behavior. If stale secrets still exist in a file but the secure backend is now keyring, the coordinator can prefer file metadata + keyring data when reconstructing logical values.

Write planning should happen inside the composite layer. Store should not need to understand that one logical patch expands into multiple child changes.

## Open design questions

### Coordination metadata ownership

- Should backlog-cli keep secret path ownership in its own package metadata first
- Should the composite layer accept explicit path lists, a planner callback, or a small coordinator interface
- At what point would repeated usage justify promoting projection metadata into jubako itself
- Whether projection targets should be selected by type, by container field, or by explicit schema matcher
- Whether the first implementation should target a mixed child model (`Layer` + lookup store) instead of forcing every child into `Layer`

### Shared schema access

- Whether schema-building helpers should move to an internal shared package immediately
- Whether jubako should expose `reflect.StructField` directly or a safer descriptor wrapper
- Whether coordinators need the full logical config snapshot or only path lookups
- Whether cross-layer read access should include all Store layers or only the coordinated layer's child state plus resolved config snapshot

### Routed layer behavior

- Whether child planning should fail fast when one logical change expands ambiguously
- Whether coordinated writes need rollback semantics or only best-effort error propagation
- How child details should be surfaced later, if ever
- Whether coordinated execution should support safety-oriented ordering per operation kind (create/update/delete)

### Watching and reload

- How much fan-in logic belongs in the routed layer vs helper utilities
- Whether non-watchable secure backends should simply opt out of automatic reload in MVP

## Proposed phased plan

### Phase 1: coordinated composite model

- define how a composite layer reconstructs logical data from child state
- define how one logical patch expands into multiple child changes
- define the read-only coordinator context shape
- define how projectors/coordinators match subtrees structurally
- define whether children are all `Layer`s or a mix of `Layer` + lookup stores
- keep projection semantics outside jubako core tags

### Phase 2: shared schema helper

- extract current schema-building logic from Store internals into a reusable helper
- use the helper from both `Store` and coordinator-facing schema views
- avoid exposing the full internal `Schema` directly

### Phase 3: routed/composite layer MVP

- create a new layer package for coordinated composition across child layers
- implement load reconstruction, save expansion, and watch fan-in
- keep origin/details logical-layer only

### Phase 4: backlog-cli integration

- keep credential externalization rules in backlog-cli-owned metadata or coordinator logic first
- construct the coordinated layer from `credential_backend`
- keep `Credential` operations logical and backend-independent
- preserve the current `SetCredential(profileName, cred)` / `DeleteCredential(profileName)` API shape

## Current recommendation

The best current direction is:

**keep `sensitive` as the only core security tag, add a coordinated composite layer that can reconstruct and co-persist across children, and postpone any core storage metadata until jubako can define real semantics for it**

This fits the existing layer model, avoids making `Store` heavier, keeps weakly-defined routing vocabulary out of jubako core, and matches `backlog-cli` more closely than a one-path-one-child router.

## Todo

- analyze-jubako-extension: done
- refine-direction: done
- decide-storage-policy-tag: done
- extract-shared-schema-helper: pending
- prototype-routed-layer-api: in_progress
- define-watch-and-origin-mvp: pending
- map-backlog-credential-backends: pending
