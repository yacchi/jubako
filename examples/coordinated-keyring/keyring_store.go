package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/yacchi/jubako/container"
	externalstore "github.com/yacchi/jubako/helper/coordinator/external-store"
	"github.com/zalando/go-keyring"
)

type keyringStore struct {
	service string
}

func newKeyringStore(service string) *keyringStore {
	return &keyringStore{service: service}
}

func (s *keyringStore) Available() error {
	_, err := keyring.Get(s.service, "__jubako_availability_probe__")
	if errors.Is(err, keyring.ErrNotFound) || err == nil {
		return nil
	}
	return err
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

type memorySecretStore struct {
	values map[string]map[string]any
}

func newMemorySecretStore() *memorySecretStore {
	return &memorySecretStore{
		values: make(map[string]map[string]any),
	}
}

func (s *memorySecretStore) Get(ctx context.Context, c externalstore.ExternalContext[*Credential]) (map[string]any, error) {
	value, ok := s.values[c.ExternalKey]
	if !ok {
		return nil, externalstore.NewNotExistError(c.ExternalKey, errors.New("secret not found"))
	}
	return container.DeepCopyMap(value), nil
}

func (s *memorySecretStore) Set(ctx context.Context, c externalstore.ExternalContext[*Credential], value map[string]any) error {
	s.values[c.ExternalKey] = container.DeepCopyMap(value)
	return nil
}

func (s *memorySecretStore) Delete(ctx context.Context, c externalstore.ExternalContext[*Credential]) error {
	delete(s.values, c.ExternalKey)
	return nil
}
