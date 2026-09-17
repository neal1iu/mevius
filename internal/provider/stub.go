package provider

import (
	"context"
	"encoding/json"
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

func (s *stubProvider) Descriptor() domain.ProviderDescriptor {
	switch s.providerType {
	case string(domain.ProviderTypeGitHub):
		return GitHubDescriptor
	case string(domain.ProviderTypeCloudflare):
		return CloudflareDescriptor
	case string(domain.ProviderTypeVercel):
		return VercelDescriptor
	default:
		return domain.ProviderDescriptor{Type: domain.ProviderType(s.providerType)}
	}
}

func (s *stubProvider) ValidateCredentials(_ context.Context, conn *domain.ProviderConnection, credential []byte) (json.RawMessage, error) {
	if len(credential) == 0 {
		return nil, &Error{
			Kind:        KindUnauthorized,
			ProviderMsg: "stub: credential is required",
		}
	}
	if strings.HasPrefix(string(credential), "bad") {
		return nil, &Error{
			Kind:        KindUnauthorized,
			ProviderMsg: "stub: invalid credential",
		}
	}
	return json.RawMessage(`{"account_id":"stub-account-` + s.providerType + `"}`), nil
}