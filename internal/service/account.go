package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"mevius/internal/crypto"
	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"

	"github.com/google/uuid"
)

type AccountService struct {
	store    *store.Queries
	key      [32]byte
	registry *provider.Registry
}

func NewAccountService(s *store.Queries, key [32]byte, reg *provider.Registry) *AccountService {
	return &AccountService{store: s, key: key, registry: reg}
}

func (s *AccountService) AddAccount(ctx context.Context, providerType, label, token string) (*domain.ProviderAccount, error) {
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}

	p := s.registry.Get(providerType)
	if p == nil {
		return nil, fmt.Errorf("unsupported provider: %s", providerType)
	}

	tmp := &domain.ProviderAccount{TokenEncrypted: token}
	meta, err := p.ValidateCredentials(ctx, tmp)
	if err != nil {
		return nil, err
	}

	encrypted := crypto.Encrypt(s.key, token)

	metaRaw, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal meta: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	account := &domain.ProviderAccount{
		ID:             uuid.New().String(),
		Provider:       domain.ProviderType(providerType),
		Label:          label,
		TokenEncrypted: encrypted,
		Meta:           meta,
		CreatedAt:      now,
	}

	if err := s.store.InsertProviderAccount(ctx, store.InsertProviderAccountParams{
		ID:             account.ID,
		Provider:       string(account.Provider),
		Label:          account.Label,
		EncryptedToken: encrypted,
		MetaJson:       string(metaRaw),
		CreatedAt:      now,
	}); err != nil {
		return nil, fmt.Errorf("insert account: %w", err)
	}

	account.TokenEncrypted = ""
	return account, nil
}

func (s *AccountService) ListAccounts(ctx context.Context) ([]domain.ProviderAccount, error) {
	rows, err := s.store.ListProviderAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	accounts := make([]domain.ProviderAccount, 0, len(rows))
	for _, r := range rows {
		acct := toDomainAccount(r)
		accounts = append(accounts, acct)
	}
	return accounts, nil
}

func (s *AccountService) GetAccount(ctx context.Context, id string) (*domain.ProviderAccount, error) {
	row, err := s.store.GetProviderAccount(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	acct := toDomainAccount(row)
	return &acct, nil
}

func (s *AccountService) DeleteAccount(ctx context.Context, id string) error {
	if err := s.store.DeleteProviderAccount(ctx, id); err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	return nil
}

func toDomainAccount(r store.ProviderAccount) domain.ProviderAccount {
	var meta domain.AccountMeta
	json.Unmarshal([]byte(r.MetaJson), &meta)

	return domain.ProviderAccount{
		ID:             r.ID,
		Provider:       domain.ProviderType(r.Provider),
		Label:          r.Label,
		TokenEncrypted: "",
		Meta:           meta,
		CreatedAt:      r.CreatedAt,
	}
}