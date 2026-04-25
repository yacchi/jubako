package main

import (
	"context"
	"encoding/json"
	"errors"

	externalstore "github.com/yacchi/jubako/helper/coordinator/external-store"
	"github.com/zalando/go-keyring"
)

type keyringStore struct {
	service string
}

func newKeyringStore(service string) *keyringStore {
	return &keyringStore{service: service}
}

func (s *keyringStore) Get(ctx context.Context, c externalstore.ExternalContext[*Credential]) (map[string]any, error) {
	secret, err := keyring.Get(s.service, c.ExternalKey)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, externalstore.NewNotExistError(c.ExternalKey, err)
		}
		return nil, err
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(secret), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *keyringStore) Set(ctx context.Context, c externalstore.ExternalContext[*Credential], value map[string]any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return keyring.Set(s.service, c.ExternalKey, string(data))
}

func (s *keyringStore) Delete(ctx context.Context, c externalstore.ExternalContext[*Credential]) error {
	err := keyring.Delete(s.service, c.ExternalKey)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
