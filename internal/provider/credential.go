package provider

import (
	"context"

	"mevius/internal/crypto"
	"mevius/internal/domain"
)

type credentialStore struct {
	key   [32]byte
	store CredentialStoreProvider
}

type CredentialStoreProvider interface {
	GetProviderConnectionCredential(ctx context.Context, id string) (string, error)
}

func NewCredentialStore(key [32]byte, store CredentialStoreProvider) domain.CredentialStore {
	return &credentialStore{key: key, store: store}
}

func (s *credentialStore) Resolve(ctx context.Context, ref string) ([]byte, error) {
	enc, err := s.store.GetProviderConnectionCredential(ctx, ref)
	if err != nil {
		return nil, err
	}
	decrypted, err := crypto.Decrypt(s.key, enc)
	if err != nil {
		return nil, err
	}
	return []byte(decrypted), nil
}