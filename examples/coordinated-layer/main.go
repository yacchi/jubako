// Example: coordinated-layer
//
// This example demonstrates a handwritten coordinator built with coordinated.New
// that exposes one logical configuration layer while coordinating two physical stores:
//   - document metadata
//   - an external secret store (simulated in memory here)
//
// The application owns the meaning of `storage:"keyring"`. jubako only exposes
// those tags through schema helpers so the coordinator can split and hydrate the
// tagged fields during Load/Save.
//
// Run with: go run ./examples/coordinated-layer
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/layer/mapdata"
)

func main() {
	ctx := context.Background()

	secretStore := newMemorySecretStore()
	metadata := newCredentialsMetadata(map[string]any{
		"credential": map[string]any{
			"default": map[string]any{
				"base_url":     "https://example.backlog.jp",
				"access_token": "legacy-plain-token",
			},
		},
	})
	credentialsLayer, err := newCredentialsLayer(metadata, secretStore)
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
	if err := store.Add(credentialsLayer, jubako.WithSensitive()); err != nil {
		log.Fatal(err)
	}

	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Coordinated Layer Example ===")
	fmt.Printf("Dirty after load: %v\n", store.IsDirty())
	fmt.Printf("Logical credential (masked): %v\n", store.GetAt("/credential/default/access_token").Value)
	fmt.Printf("Logical credential (unmasked): %v\n", store.GetAtUnmasked("/credential/default/access_token").Value)
	fmt.Printf("Metadata before save: %#v\n", metadata.Data())
	fmt.Printf("Secret store before save: %#v\n", secretStore.Snapshot())

	if err := store.Save(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println()
	fmt.Println("After migration save:")
	fmt.Printf("Metadata: %#v\n", metadata.Data())
	fmt.Printf("Secret store: %#v\n", secretStore.Snapshot())

	if err := store.Set(layerCredentials, jubako.String("/credential/default/access_token", "rotated-token")); err != nil {
		log.Fatal(err)
	}
	if err := store.Save(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println()
	fmt.Println("After logical update + save:")
	fmt.Printf("Logical credential (unmasked): %v\n", store.GetAtUnmasked("/credential/default/access_token").Value)
	fmt.Printf("Metadata: %#v\n", metadata.Data())
	fmt.Printf("Secret store: %#v\n", secretStore.Snapshot())
}
