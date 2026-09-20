package provider

import (
	"context"
	"strings"

	"mevius/internal/domain"
)

type stubProvider struct {
	id         domain.ProviderID
	descriptor domain.ProviderDescriptor
	products   []ProductDriver
}
type stubProduct struct{ descriptor domain.ProductDescriptor }

func NewStubProvider(providerType string) Provider {
	var descriptor domain.ProviderDescriptor
	switch domain.ProviderID(providerType) {
	case "github":
		descriptor = GitHubDescriptor
	case "cloudflare":
		descriptor = CloudflareDescriptor
	case "vercel":
		descriptor = VercelDescriptor
	default:
		descriptor = domain.ProviderDescriptor{ID: domain.ProviderID(providerType), DisplayName: providerType}
	}
	result := &stubProvider{id: descriptor.ID, descriptor: descriptor}
	for _, product := range descriptor.Products {
		result.products = append(result.products, &stubProduct{descriptor: product})
	}
	return result
}
func (s *stubProvider) ID() domain.ProviderID                 { return s.id }
func (s *stubProvider) Descriptor() domain.ProviderDescriptor { return s.descriptor }
func (s *stubProvider) Products() []ProductDriver             { return s.products }
func (s *stubProvider) Probe(_ context.Context, _ string, credential []byte) (*domain.ProbeResult, error) {
	if len(credential) == 0 || strings.HasPrefix(string(credential), "bad") {
		return nil, &Error{Kind: KindUnauthorized, ProviderMsg: "stub: invalid credential"}
	}
	permissions := map[domain.Capability]domain.CapabilityState{}
	for _, product := range s.descriptor.Products {
		for _, capability := range product.Capabilities {
			permissions[capability] = domain.CapabilityState{Availability: domain.CapabilityAvailable}
		}
	}
	return &domain.ProbeResult{Identity: map[string]any{"provider": s.id}, Scopes: []domain.ProviderScope{{Type: "account", ID: "stub-account-" + string(s.id), Label: "Stub account"}}, Permissions: permissions}, nil
}
func (s *stubProvider) ValidateScope(ctx context.Context, endpoint string, credential []byte, scope domain.ProviderScope) (*domain.ProbeResult, error) {
	result, err := s.Probe(ctx, endpoint, credential)
	if err != nil {
		return nil, err
	}
	result.Scopes = []domain.ProviderScope{scope}
	return result, nil
}
func (s *stubProduct) Descriptor() domain.ProductDescriptor { return s.descriptor }
