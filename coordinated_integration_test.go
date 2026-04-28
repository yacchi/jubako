package jubako

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/yacchi/jubako/container"
	"github.com/yacchi/jubako/document"
	externalstore "github.com/yacchi/jubako/helper/coordinator/external-store"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/layer/mapdata"
)

type coordinatedTestAuth struct {
	CredentialBackend string `json:"credential_backend"`
}

type coordinatedTestProfile struct {
	SpaceID string `json:"space_id"`
}

type coordinatedTestCredential struct {
	BaseURL     string `json:"base_url"`
	AccessToken string `json:"access_token" jubako:"sensitive" storage:"keyring"`
	SecretRef   string `json:"secret_ref,omitempty"`
}

type coordinatedTestConfig struct {
	Auth       coordinatedTestAuth                   `json:"auth"`
	Profile    map[string]*coordinatedTestProfile    `json:"profile"`
	Credential map[string]*coordinatedTestCredential `json:"credential"`
}

type testSecretStore struct {
	values      map[string]map[string]any
	lastGet     externalstore.ExternalContext[*coordinatedTestCredential]
	lastSet     externalstore.ExternalContext[*coordinatedTestCredential]
	lastDelete  externalstore.ExternalContext[*coordinatedTestCredential]
	getCalls    int
	setCalls    int
	deleteCalls int
	getErr      error
	setErr      error
	deleteErr   error
}

func newTestSecretStore() *testSecretStore {
	return &testSecretStore{values: make(map[string]map[string]any)}
}

func (s *testSecretStore) Get(ctx context.Context, c externalstore.ExternalContext[*coordinatedTestCredential]) (map[string]any, error) {
	s.lastGet = c
	s.getCalls++
	if s.getErr != nil {
		return nil, s.getErr
	}
	value, ok := s.values[c.ExternalKey]
	if !ok {
		return nil, externalstore.NewNotExistError(c.ExternalKey, errors.New("missing test secret"))
	}
	return container.DeepCopyMap(value), nil
}

func (s *testSecretStore) Set(ctx context.Context, c externalstore.ExternalContext[*coordinatedTestCredential], value map[string]any) error {
	s.lastSet = c
	s.setCalls++
	if s.setErr != nil {
		return s.setErr
	}
	s.values[c.ExternalKey] = container.DeepCopyMap(value)
	return nil
}

func (s *testSecretStore) Delete(ctx context.Context, c externalstore.ExternalContext[*coordinatedTestCredential]) error {
	s.lastDelete = c
	s.deleteCalls++
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.values, c.ExternalKey)
	return nil
}

type failingMetadataLayer struct {
	*mapdata.Layer
	saveErr error
}

func (l *failingMetadataLayer) Save(ctx context.Context, changeset document.JSONPatchSet) error {
	if l.saveErr != nil {
		return l.saveErr
	}
	return l.Layer.Save(ctx, changeset)
}

func coordinatedTestRoute(ctx externalstore.RouteContext[*coordinatedTestCredential]) (externalstore.Route, error) {
	if !ctx.HasEntry || ctx.Entry == nil {
		return externalstore.Route{}, fmt.Errorf("entry missing for %q", ctx.Key)
	}
	if ctx.Entry.BaseURL == "" {
		return externalstore.Route{}, fmt.Errorf("base_url missing for %q", ctx.Key)
	}

	backendValue, _ := jsonptr.GetPath(ctx.Logical, "/auth/credential_backend")
	backend, _ := backendValue.(string)
	ref := ctx.ExistingRef
	if ref == "" {
		ref = "cred-" + ctx.Key
	}
	spaceIDPath := "/profile/" + jsonptr.Escape(ctx.Key) + "/space_id"
	spaceIDValue, ok := jsonptr.GetPath(ctx.Logical, spaceIDPath)
	if !ok {
		return externalstore.Route{}, fmt.Errorf("space_id not found at %q", spaceIDPath)
	}
	spaceID, ok := spaceIDValue.(string)
	if !ok || spaceID == "" {
		return externalstore.Route{}, fmt.Errorf("space_id at %q is %T", spaceIDPath, spaceIDValue)
	}
	return externalstore.Route{
		UseExternal: backend == "keyring",
		Ref:         ref,
	}, nil
}

func coordinatedTestRouteWithMutableExternalKey(ctx externalstore.RouteContext[*coordinatedTestCredential]) (externalstore.Route, error) {
	route, err := coordinatedTestRoute(ctx)
	if err != nil {
		return externalstore.Route{}, err
	}
	spaceIDPath := "/profile/" + jsonptr.Escape(ctx.Key) + "/space_id"
	spaceIDValue, ok := jsonptr.GetPath(ctx.Logical, spaceIDPath)
	if !ok {
		return externalstore.Route{}, fmt.Errorf("space_id not found at %q", spaceIDPath)
	}
	spaceID, ok := spaceIDValue.(string)
	if !ok || spaceID == "" {
		return externalstore.Route{}, fmt.Errorf("space_id at %q is %T", spaceIDPath, spaceIDValue)
	}
	route.ExternalKey = "backlog/" + spaceID + "/" + route.Ref
	return route, nil
}

func newCoordinatedTestLayer(
	t *testing.T,
	metadata layer.Layer,
	secretStore externalstore.SecretStore[*coordinatedTestCredential],
	route externalstore.RouteFunc[*coordinatedTestCredential],
) layer.Layer {
	t.Helper()

	credentials, err := externalstore.NewMap[*coordinatedTestCredential]("credentials", externalstore.MapConfig[*coordinatedTestCredential]{
		RootPath:         "/credential",
		Metadata:         metadata,
		External:         secretStore,
		RefPath:          "/secret_ref",
		ExternalTagKey:   "storage",
		ExternalTagValue: "keyring",
		RouteForEntry:    route,
	})
	if err != nil {
		t.Fatalf("NewMap() error = %v", err)
	}
	return credentials
}

func newCoordinatedTestStore(
	t *testing.T,
	backend string,
	profiles map[string]any,
	credentials layer.Layer,
) *Store[coordinatedTestConfig] {
	t.Helper()

	store := New[coordinatedTestConfig]()
	if err := store.Add(mapdata.New("auth", map[string]any{
		"auth": map[string]any{"credential_backend": backend},
	})); err != nil {
		t.Fatalf("Add(auth) error = %v", err)
	}
	if err := store.Add(mapdata.New("profile", map[string]any{
		"profile": profiles,
	})); err != nil {
		t.Fatalf("Add(profile) error = %v", err)
	}
	if err := store.Add(credentials, WithSensitive()); err != nil {
		t.Fatalf("Add(credentials) error = %v", err)
	}
	return store
}

func TestCoordinatedLayer_SaveMigratesAndUpdatesTaggedSecrets(t *testing.T) {
	ctx := context.Background()
	secretStore := newTestSecretStore()
	metadata := mapdata.New("credentials-meta", map[string]any{
		"credential": map[string]any{
			"default": map[string]any{
				"base_url":     "https://example.backlog.jp",
				"access_token": "legacy-plain-token",
			},
		},
	})

	credentials := newCoordinatedTestLayer(t, metadata, secretStore, coordinatedTestRoute)
	store := newCoordinatedTestStore(t, "keyring", map[string]any{
		"default": map[string]any{
			"space_id": "space-a",
		},
	}, credentials)

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !store.IsDirty() {
		t.Fatal("IsDirty() = false, want true after projection-dirty migration detection")
	}
	if got := store.Get().Credential["default"].AccessToken; got != "legacy-plain-token" {
		t.Fatalf("AccessToken after Load = %q, want %q", got, "legacy-plain-token")
	}

	if err := store.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	currentMetadata := metadata.Data()
	got, ok := jsonptr.GetPath(currentMetadata, "/credential/default/secret_ref")
	if !ok || got != "cred-default" {
		t.Fatalf("metadata secret_ref = (%v, %v), want (%q, true)", got, ok, "cred-default")
	}
	if _, ok := jsonptr.GetPath(currentMetadata, "/credential/default/access_token"); ok {
		t.Fatal("metadata access_token still present after migration")
	}

	secret, err := secretStore.Get(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-default",
	})
	if err != nil {
		t.Fatalf("secretStore.Get() error = %v", err)
	}
	if got, ok := jsonptr.GetPath(secret, "/access_token"); !ok || got != "legacy-plain-token" {
		t.Fatalf("secret access_token = (%v, %v), want (%q, true)", got, ok, "legacy-plain-token")
	}

	if err := store.Set("credentials", String("/credential/default/access_token", "rotated-token")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Save(ctx); err != nil {
		t.Fatalf("Save() after update error = %v", err)
	}

	if !secretStore.lastSet.HasAfter || secretStore.lastSet.After == nil {
		t.Fatal("lastSet.After missing, want logical entry context")
	}
	if got := secretStore.lastSet.After.BaseURL; got != "https://example.backlog.jp" {
		t.Fatalf("lastSet.After.BaseURL = %q, want %q", got, "https://example.backlog.jp")
	}
	if got := secretStore.lastSet.Ref; got != "cred-default" {
		t.Fatalf("lastSet.Ref = %q, want %q", got, "cred-default")
	}
	if got := secretStore.lastSet.EntryPath; got != "/credential/default" {
		t.Fatalf("lastSet.EntryPath = %q, want %q", got, "/credential/default")
	}

	secret, err = secretStore.Get(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-default",
	})
	if err != nil {
		t.Fatalf("secretStore.Get() after update error = %v", err)
	}
	if got, ok := jsonptr.GetPath(secret, "/access_token"); !ok || got != "rotated-token" {
		t.Fatalf("secret access_token after update = (%v, %v), want (%q, true)", got, ok, "rotated-token")
	}
}

func TestCoordinatedLayer_LoadIgnoresMissingExternalPayload(t *testing.T) {
	ctx := context.Background()
	secretStore := newTestSecretStore()
	metadata := mapdata.New("credentials-meta", map[string]any{
		"credential": map[string]any{
			"default": map[string]any{
				"base_url":   "https://example.backlog.jp",
				"secret_ref": "cred-default",
			},
		},
	})

	credentials := newCoordinatedTestLayer(t, metadata, secretStore, coordinatedTestRoute)
	store := newCoordinatedTestStore(t, "keyring", map[string]any{
		"default": map[string]any{
			"space_id": "space-a",
		},
	}, credentials)

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cred := store.Get().Credential["default"]
	if cred == nil {
		t.Fatal("Credential[default] = nil, want entry")
	}
	if cred.SecretRef != "cred-default" {
		t.Fatalf("SecretRef = %q, want %q", cred.SecretRef, "cred-default")
	}
	if cred.AccessToken != "" {
		t.Fatalf("AccessToken = %q, want empty on missing external payload", cred.AccessToken)
	}
	if store.IsDirty() {
		t.Fatal("IsDirty() = true, want false when only external payload is missing")
	}
}

func TestCoordinatedLayer_SaveMigratesExternalToLocal(t *testing.T) {
	ctx := context.Background()
	secretStore := newTestSecretStore()
	if err := secretStore.Set(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-default",
	}, map[string]any{
		"access_token": "from-keyring",
	}); err != nil {
		t.Fatalf("secretStore.Set() error = %v", err)
	}

	metadata := mapdata.New("credentials-meta", map[string]any{
		"credential": map[string]any{
			"default": map[string]any{
				"base_url":   "https://example.backlog.jp",
				"secret_ref": "cred-default",
			},
		},
	})

	credentials := newCoordinatedTestLayer(t, metadata, secretStore, coordinatedTestRoute)
	store := newCoordinatedTestStore(t, "file", map[string]any{
		"default": map[string]any{
			"space_id": "space-a",
		},
	}, credentials)

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cred := store.Get().Credential["default"]
	if cred == nil {
		t.Fatal("Credential[default] = nil, want entry")
	}
	if cred.AccessToken != "from-keyring" {
		t.Fatalf("AccessToken after Load = %q, want %q", cred.AccessToken, "from-keyring")
	}
	if !store.IsDirty() {
		t.Fatal("IsDirty() = false, want true for external-to-local reprojection")
	}

	if err := store.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	currentMetadata := metadata.Data()
	got, ok := jsonptr.GetPath(currentMetadata, "/credential/default/access_token")
	if !ok || got != "from-keyring" {
		t.Fatalf("metadata access_token = (%v, %v), want (%q, true)", got, ok, "from-keyring")
	}
	if _, ok := jsonptr.GetPath(currentMetadata, "/credential/default/secret_ref"); ok {
		t.Fatal("metadata secret_ref still present after external-to-local migration")
	}
	if !secretStore.lastDelete.HasAfter || secretStore.lastDelete.After == nil {
		t.Fatal("lastDelete.After missing, want logical entry context")
	}
	if got := secretStore.lastDelete.After.AccessToken; got != "from-keyring" {
		t.Fatalf("lastDelete.After.AccessToken = %q, want %q", got, "from-keyring")
	}
	if _, err := secretStore.Get(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-default",
	}); !errors.Is(err, externalstore.ErrNotExist) {
		t.Fatalf("secretStore.Get() error = %v, want ErrNotExist after cleanup", err)
	}

	cred = store.Get().Credential["default"]
	if cred == nil {
		t.Fatal("Credential[default] after Save = nil, want entry")
	}
	if cred.AccessToken != "from-keyring" {
		t.Fatalf("AccessToken after Save = %q, want %q", cred.AccessToken, "from-keyring")
	}
}

func TestCoordinatedLayer_SaveHandlesMixedMultiEntryProjection(t *testing.T) {
	ctx := context.Background()
	secretStore := newTestSecretStore()
	if err := secretStore.Set(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-default",
	}, map[string]any{
		"access_token": "token-default",
	}); err != nil {
		t.Fatalf("secretStore.Set(default) error = %v", err)
	}
	if err := secretStore.Set(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-secondary",
	}, map[string]any{
		"access_token": "token-secondary",
	}); err != nil {
		t.Fatalf("secretStore.Set(secondary) error = %v", err)
	}

	metadata := mapdata.New("credentials-meta", map[string]any{
		"credential": map[string]any{
			"default": map[string]any{
				"base_url":   "https://default.backlog.jp",
				"secret_ref": "cred-default",
			},
			"secondary": map[string]any{
				"base_url":   "https://secondary.backlog.jp",
				"secret_ref": "cred-secondary",
			},
		},
	})

	route := func(ctx externalstore.RouteContext[*coordinatedTestCredential]) (externalstore.Route, error) {
		route, err := coordinatedTestRoute(ctx)
		if err != nil {
			return externalstore.Route{}, err
		}
		if ctx.Key == "secondary" {
			route.UseExternal = false
		}
		return route, nil
	}

	credentials := newCoordinatedTestLayer(t, metadata, secretStore, route)
	store := newCoordinatedTestStore(t, "keyring", map[string]any{
		"default": map[string]any{
			"space_id": "space-a",
		},
		"secondary": map[string]any{
			"space_id": "space-b",
		},
	}, credentials)

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !store.IsDirty() {
		t.Fatal("IsDirty() = false, want true for secondary external-to-local reprojection")
	}

	if err := store.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	currentMetadata := metadata.Data()
	if got, ok := jsonptr.GetPath(currentMetadata, "/credential/default/secret_ref"); !ok || got != "cred-default" {
		t.Fatalf("default secret_ref = (%v, %v), want (%q, true)", got, ok, "cred-default")
	}
	if _, ok := jsonptr.GetPath(currentMetadata, "/credential/default/access_token"); ok {
		t.Fatal("default metadata access_token still present, want external-only storage")
	}
	if got, ok := jsonptr.GetPath(currentMetadata, "/credential/secondary/access_token"); !ok || got != "token-secondary" {
		t.Fatalf("secondary access_token = (%v, %v), want (%q, true)", got, ok, "token-secondary")
	}
	if _, ok := jsonptr.GetPath(currentMetadata, "/credential/secondary/secret_ref"); ok {
		t.Fatal("secondary secret_ref still present after local reprojection")
	}

	defaultSecret, err := secretStore.Get(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-default",
	})
	if err != nil {
		t.Fatalf("secretStore.Get(default) error = %v", err)
	}
	if got, ok := jsonptr.GetPath(defaultSecret, "/access_token"); !ok || got != "token-default" {
		t.Fatalf("default secret access_token = (%v, %v), want (%q, true)", got, ok, "token-default")
	}
	if _, err := secretStore.Get(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-secondary",
	}); !errors.Is(err, externalstore.ErrNotExist) {
		t.Fatalf("secretStore.Get(secondary) error = %v, want ErrNotExist after cleanup", err)
	}
}

func TestCoordinatedLayer_ExternalKeyRouteDriftNeedsEntrySave(t *testing.T) {
	ctx := context.Background()
	secretStore := newTestSecretStore()
	if err := secretStore.Set(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "backlog/space-a/cred-default",
	}, map[string]any{
		"access_token": "from-space-a",
	}); err != nil {
		t.Fatalf("secretStore.Set() error = %v", err)
	}

	metadata := mapdata.New("credentials-meta", map[string]any{
		"credential": map[string]any{
			"default": map[string]any{
				"base_url":   "https://example.backlog.jp",
				"secret_ref": "cred-default",
			},
		},
	})

	credentials := newCoordinatedTestLayer(t, metadata, secretStore, coordinatedTestRouteWithMutableExternalKey)
	store := newCoordinatedTestStore(t, "keyring", map[string]any{
		"default": map[string]any{
			"space_id": "space-a",
		},
	}, credentials)

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := store.Get().Credential["default"].AccessToken; got != "from-space-a" {
		t.Fatalf("AccessToken after Load = %q, want %q", got, "from-space-a")
	}

	if err := store.Set("profile", String("/profile/default/space_id", "space-b")); err != nil {
		t.Fatalf("Set(profile) error = %v", err)
	}
	if err := store.SaveLayer(ctx, "profile"); err != nil {
		t.Fatalf("SaveLayer(profile) error = %v", err)
	}
	if err := store.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	cred := store.Get().Credential["default"]
	if cred == nil {
		t.Fatal("Credential[default] = nil, want entry")
	}
	if cred.AccessToken != "" {
		t.Fatalf("AccessToken after route drift reload = %q, want empty under current unsupported contract", cred.AccessToken)
	}
	if store.IsDirty() {
		t.Fatal("IsDirty() = true, want false when only ExternalKey route drift occurred")
	}
}

func TestCoordinatedLayer_SaveReturnsExternalSetFailureWithoutMetadataMutation(t *testing.T) {
	ctx := context.Background()
	secretStore := newTestSecretStore()
	setErr := errors.New("set failed")
	secretStore.setErr = setErr
	metadata := mapdata.New("credentials-meta", map[string]any{
		"credential": map[string]any{
			"default": map[string]any{
				"base_url":     "https://example.backlog.jp",
				"access_token": "legacy-plain-token",
			},
		},
	})

	credentials := newCoordinatedTestLayer(t, metadata, secretStore, coordinatedTestRoute)
	store := newCoordinatedTestStore(t, "keyring", map[string]any{
		"default": map[string]any{
			"space_id": "space-a",
		},
	}, credentials)

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	err := store.Save(ctx)
	if !errors.Is(err, setErr) {
		t.Fatalf("Save() error = %v, want %v", err, setErr)
	}
	currentMetadata := metadata.Data()
	if got, ok := jsonptr.GetPath(currentMetadata, "/credential/default/access_token"); !ok || got != "legacy-plain-token" {
		t.Fatalf("metadata access_token = (%v, %v), want (%q, true)", got, ok, "legacy-plain-token")
	}
	if _, ok := jsonptr.GetPath(currentMetadata, "/credential/default/secret_ref"); ok {
		t.Fatal("metadata secret_ref present after failed external set")
	}
	if _, err := secretStore.Get(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-default",
	}); !errors.Is(err, externalstore.ErrNotExist) {
		t.Fatalf("secretStore.Get() error = %v, want ErrNotExist", err)
	}
	if !store.IsDirty() {
		t.Fatal("IsDirty() = false, want true after failed save")
	}
}

func TestCoordinatedLayer_SaveReturnsExternalDeleteFailureWithoutMetadataMutation(t *testing.T) {
	ctx := context.Background()
	secretStore := newTestSecretStore()
	deleteErr := errors.New("delete failed")
	if err := secretStore.Set(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-default",
	}, map[string]any{
		"access_token": "from-keyring",
	}); err != nil {
		t.Fatalf("secretStore.Set() error = %v", err)
	}
	secretStore.deleteErr = deleteErr

	metadata := mapdata.New("credentials-meta", map[string]any{
		"credential": map[string]any{
			"default": map[string]any{
				"base_url":   "https://example.backlog.jp",
				"secret_ref": "cred-default",
			},
		},
	})

	credentials := newCoordinatedTestLayer(t, metadata, secretStore, coordinatedTestRoute)
	store := newCoordinatedTestStore(t, "file", map[string]any{
		"default": map[string]any{
			"space_id": "space-a",
		},
	}, credentials)

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	err := store.Save(ctx)
	if !errors.Is(err, deleteErr) {
		t.Fatalf("Save() error = %v, want %v", err, deleteErr)
	}
	currentMetadata := metadata.Data()
	if _, ok := jsonptr.GetPath(currentMetadata, "/credential/default/access_token"); ok {
		t.Fatal("metadata access_token present after failed external delete")
	}
	if got, ok := jsonptr.GetPath(currentMetadata, "/credential/default/secret_ref"); !ok || got != "cred-default" {
		t.Fatalf("metadata secret_ref = (%v, %v), want (%q, true)", got, ok, "cred-default")
	}
	secret, err := secretStore.Get(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-default",
	})
	if err != nil {
		t.Fatalf("secretStore.Get() error = %v", err)
	}
	if got, ok := jsonptr.GetPath(secret, "/access_token"); !ok || got != "from-keyring" {
		t.Fatalf("secret access_token = (%v, %v), want (%q, true)", got, ok, "from-keyring")
	}
	if !store.IsDirty() {
		t.Fatal("IsDirty() = false, want true after failed save")
	}
}

func TestCoordinatedLayer_SaveReturnsMetadataFailureAfterExternalWrite(t *testing.T) {
	ctx := context.Background()
	secretStore := newTestSecretStore()
	saveErr := errors.New("metadata save failed")
	metadata := &failingMetadataLayer{
		Layer: mapdata.New("credentials-meta", map[string]any{
			"credential": map[string]any{
				"default": map[string]any{
					"base_url":     "https://example.backlog.jp",
					"access_token": "legacy-plain-token",
				},
			},
		}),
		saveErr: saveErr,
	}

	credentials := newCoordinatedTestLayer(t, metadata, secretStore, coordinatedTestRoute)
	store := newCoordinatedTestStore(t, "keyring", map[string]any{
		"default": map[string]any{
			"space_id": "space-a",
		},
	}, credentials)

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	err := store.Save(ctx)
	if !errors.Is(err, saveErr) {
		t.Fatalf("Save() error = %v, want %v", err, saveErr)
	}
	secret, err := secretStore.Get(ctx, externalstore.ExternalContext[*coordinatedTestCredential]{
		ExternalKey: "cred-default",
	})
	if err != nil {
		t.Fatalf("secretStore.Get() error = %v", err)
	}
	if got, ok := jsonptr.GetPath(secret, "/access_token"); !ok || got != "legacy-plain-token" {
		t.Fatalf("secret access_token = (%v, %v), want (%q, true)", got, ok, "legacy-plain-token")
	}
	currentMetadata := metadata.Data()
	if got, ok := jsonptr.GetPath(currentMetadata, "/credential/default/access_token"); !ok || got != "legacy-plain-token" {
		t.Fatalf("metadata access_token = (%v, %v), want (%q, true)", got, ok, "legacy-plain-token")
	}
	if _, ok := jsonptr.GetPath(currentMetadata, "/credential/default/secret_ref"); ok {
		t.Fatal("metadata secret_ref present after failed metadata save")
	}
	if !store.IsDirty() {
		t.Fatal("IsDirty() = false, want true after failed save")
	}
}
