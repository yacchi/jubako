package jubako

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/yacchi/jubako/container"
	externalstore "github.com/yacchi/jubako/helper/coordinator/external-store"
	"github.com/yacchi/jubako/jsonptr"
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
}

func newTestSecretStore() *testSecretStore {
	return &testSecretStore{values: make(map[string]map[string]any)}
}

func (s *testSecretStore) Get(ctx context.Context, c externalstore.ExternalContext[*coordinatedTestCredential]) (map[string]any, error) {
	s.lastGet = c
	s.getCalls++
	value, ok := s.values[c.ExternalKey]
	if !ok {
		return nil, externalstore.NewNotExistError(c.ExternalKey, errors.New("missing test secret"))
	}
	return container.DeepCopyMap(value), nil
}

func (s *testSecretStore) Set(ctx context.Context, c externalstore.ExternalContext[*coordinatedTestCredential], value map[string]any) error {
	s.lastSet = c
	s.setCalls++
	s.values[c.ExternalKey] = container.DeepCopyMap(value)
	return nil
}

func (s *testSecretStore) Delete(ctx context.Context, c externalstore.ExternalContext[*coordinatedTestCredential]) error {
	s.lastDelete = c
	s.deleteCalls++
	delete(s.values, c.ExternalKey)
	return nil
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

	credentials, err := externalstore.NewMap[*coordinatedTestCredential]("credentials", externalstore.MapConfig[*coordinatedTestCredential]{
		RootPath:         "/credential",
		Metadata:         metadata,
		External:         secretStore,
		RefPath:          "/secret_ref",
		ExternalTagKey:   "storage",
		ExternalTagValue: "keyring",
		RouteForEntry:    coordinatedTestRoute,
	})
	if err != nil {
		t.Fatalf("NewMap() error = %v", err)
	}

	store := New[coordinatedTestConfig]()
	if err := store.Add(mapdata.New("auth", map[string]any{
		"auth": map[string]any{"credential_backend": "keyring"},
	})); err != nil {
		t.Fatalf("Add(auth) error = %v", err)
	}
	if err := store.Add(mapdata.New("profile", map[string]any{
		"profile": map[string]any{
			"default": map[string]any{
				"space_id": "space-a",
			},
		},
	})); err != nil {
		t.Fatalf("Add(profile) error = %v", err)
	}
	if err := store.Add(credentials, WithSensitive()); err != nil {
		t.Fatalf("Add(credentials) error = %v", err)
	}

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

	credentials, err := externalstore.NewMap[*coordinatedTestCredential]("credentials", externalstore.MapConfig[*coordinatedTestCredential]{
		RootPath:         "/credential",
		Metadata:         metadata,
		External:         secretStore,
		RefPath:          "/secret_ref",
		ExternalTagKey:   "storage",
		ExternalTagValue: "keyring",
		RouteForEntry:    coordinatedTestRoute,
	})
	if err != nil {
		t.Fatalf("NewMap() error = %v", err)
	}

	store := New[coordinatedTestConfig]()
	if err := store.Add(mapdata.New("auth", map[string]any{
		"auth": map[string]any{"credential_backend": "keyring"},
	})); err != nil {
		t.Fatalf("Add(auth) error = %v", err)
	}
	if err := store.Add(mapdata.New("profile", map[string]any{
		"profile": map[string]any{
			"default": map[string]any{
				"space_id": "space-a",
			},
		},
	})); err != nil {
		t.Fatalf("Add(profile) error = %v", err)
	}
	if err := store.Add(credentials, WithSensitive()); err != nil {
		t.Fatalf("Add(credentials) error = %v", err)
	}

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

	credentials, err := externalstore.NewMap[*coordinatedTestCredential]("credentials", externalstore.MapConfig[*coordinatedTestCredential]{
		RootPath:         "/credential",
		Metadata:         metadata,
		External:         secretStore,
		RefPath:          "/secret_ref",
		ExternalTagKey:   "storage",
		ExternalTagValue: "keyring",
		RouteForEntry:    coordinatedTestRoute,
	})
	if err != nil {
		t.Fatalf("NewMap() error = %v", err)
	}

	store := New[coordinatedTestConfig]()
	if err := store.Add(mapdata.New("auth", map[string]any{
		"auth": map[string]any{"credential_backend": "file"},
	})); err != nil {
		t.Fatalf("Add(auth) error = %v", err)
	}
	if err := store.Add(mapdata.New("profile", map[string]any{
		"profile": map[string]any{
			"default": map[string]any{
				"space_id": "space-a",
			},
		},
	})); err != nil {
		t.Fatalf("Add(profile) error = %v", err)
	}
	if err := store.Add(credentials, WithSensitive()); err != nil {
		t.Fatalf("Add(credentials) error = %v", err)
	}

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
