// Example: coordinated-keyring
//
// This example demonstrates helper/coordinator/external-store with a real OS
// keyring adapter. The metadata layer remains in-memory for brevity, while the
// access token is migrated into the keyring and updated there on later saves.
//
// If the OS keyring is unavailable (for example in headless CI), the example
// falls back to an in-memory secret store so it still compiles and runs.
//
// The example derives the initial ref from user_id@space_id so the stored key is
// readable in keyring UIs. ExternalKey is omitted, so the helper uses Ref as the
// backend key. After the first save, ExistingRef keeps that value stable to avoid
// route drift if user_id or space_id changes later.
//
// The demo entry is removed from the keyring before the process exits.
//
// Run with: go run ./examples/coordinated-keyring
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/yacchi/jubako"
	externalstore "github.com/yacchi/jubako/helper/coordinator/external-store"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer/mapdata"
)

func main() {
	ctx := context.Background()
	secretStore, backendName := newExampleSecretStore()
	const cleanupKey = "example-user@example-space"
	defer func() {
		_ = secretStore.Delete(ctx, externalstore.ExternalContext[*Credential]{
			ExternalKey: cleanupKey,
		})
	}()

	metadata := mapdata.New("credentials-metadata", map[string]any{
		"credential": map[string]any{
			"default": map[string]any{
				"base_url":     "https://example.backlog.jp",
				"user_id":      "example-user",
				"access_token": "legacy-plain-token",
			},
		},
	})

	credentials, err := externalstore.NewMap[*Credential](layerCredentials, externalstore.MapConfig[*Credential]{
		RootPath:         pathCredential,
		Metadata:         metadata,
		External:         secretStore,
		RefPath:          "/secret_ref",
		ExternalTagKey:   "storage",
		ExternalTagValue: "keyring",
		RouteForEntry: func(ctx externalstore.RouteContext[*Credential]) (externalstore.Route, error) {
			backendValue, _ := jsonptr.GetPath(ctx.Logical, "/auth/credential_backend")
			backend, _ := backendValue.(string)
			ref := ctx.ExistingRef
			if ref == "" {
				if !ctx.HasEntry || ctx.Entry == nil || ctx.Entry.UserID == "" {
					return externalstore.Route{}, fmt.Errorf("user_id is required for %q", ctx.Key)
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
				ref = ctx.Entry.UserID + "@" + spaceID
			}
			return externalstore.Route{
				UseExternal: backend == "keyring",
				Ref:         ref,
			}, nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	store := jubako.New[Config](jubako.WithSensitiveMaskString("********"))

	if err := store.Add(mapdata.New(layerAuth, map[string]any{
		"auth": map[string]any{
			"credential_backend": "keyring",
		},
	})); err != nil {
		log.Fatal(err)
	}
	if err := store.Add(mapdata.New("profile", map[string]any{
		"profile": map[string]any{
			"default": map[string]any{
				"space_id": "example-space",
			},
		},
	})); err != nil {
		log.Fatal(err)
	}
	if err := store.Add(credentials, jubako.WithSensitive()); err != nil {
		log.Fatal(err)
	}

	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Coordinated Keyring Example ===")
	fmt.Printf("Backend: %s\n", backendName)
	fmt.Printf("Dirty after load: %v\n", store.IsDirty())
	fmt.Printf("Logical credential (masked): %v\n", store.GetAt("/credential/default/access_token").Value)
	fmt.Printf("Logical credential (unmasked): %v\n", store.GetAtUnmasked("/credential/default/access_token").Value)
	fmt.Printf("Metadata before save: %#v\n", metadata.Data())

	if err := store.Save(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println()
	fmt.Println("After migration save:")
	fmt.Printf("Metadata: %#v\n", metadata.Data())
	fmt.Printf("Logical credential (unmasked): %v\n", store.GetAtUnmasked("/credential/default/access_token").Value)

	if err := store.Set(layerCredentials, jubako.String("/credential/default/access_token", "rotated-token")); err != nil {
		log.Fatal(err)
	}
	if err := store.Save(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println()
	fmt.Println("After logical update + save:")
	fmt.Printf("Metadata: %#v\n", metadata.Data())
	fmt.Printf("Logical credential (unmasked): %v\n", store.GetAtUnmasked("/credential/default/access_token").Value)
}

func newExampleSecretStore() (externalstore.SecretStore[*Credential], string) {
	store := newKeyringStore(keyringService)
	if err := store.Available(); err == nil {
		return store, "os-keyring"
	}
	return newMemorySecretStore(), "in-memory fallback"
}
