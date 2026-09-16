package provider

import (
	"context"
	"strings"

	"mevius/internal/domain"
)

type stubProvider struct {
	providerType string
}

func NewStubProvider(providerType string) Provider {
	return &stubProvider{providerType: providerType}
}

func (s *stubProvider) Type() string {
	return s.providerType
}

func (s *stubProvider) ValidateCredentials(_ context.Context, account *domain.ProviderAccount) (domain.AccountMeta, error) {
	if account.TokenEncrypted == "" {
		return domain.AccountMeta{}, &Error{
			Kind:        KindUnauthorized,
			ProviderMsg: "stub: token is required",
		}
	}
	if strings.HasPrefix(account.TokenEncrypted, "bad") {
		return domain.AccountMeta{}, &Error{
			Kind:        KindUnauthorized,
			ProviderMsg: "stub: invalid token",
		}
	}
	return domain.AccountMeta{
		AccountID: "stub-account-" + s.providerType,
	}, nil
}

func (s *stubProvider) ListExternalResources(_ context.Context, account *domain.ProviderAccount, kind domain.ResourceKind) ([]domain.ExternalResource, error) {
	return []domain.ExternalResource{}, nil
}