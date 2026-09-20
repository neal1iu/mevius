package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

type CloudflareProvider struct {
	workers *workerDriver
	pages   *pageDriver
	dns     *dnsDriver
}

func NewProvider() *CloudflareProvider {
	p := &CloudflareProvider{}
	p.workers, p.pages, p.dns = &workerDriver{p}, &pageDriver{p}, &dnsDriver{p}
	return p
}

func (p *CloudflareProvider) ID() domain.ProviderID { return "cloudflare" }
func (p *CloudflareProvider) Descriptor() domain.ProviderDescriptor {
	return provider.CloudflareDescriptor
}
func (p *CloudflareProvider) Products() []provider.ProductDriver {
	return []provider.ProductDriver{p.workers, p.pages, p.dns}
}

func endpoint(v string) string {
	if v == "" {
		return "https://api.cloudflare.com/client/v4"
	}
	return strings.TrimRight(v, "/")
}

type response struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (p *CloudflareProvider) request(ctx context.Context, method, ep, token, path string, value any) ([]byte, error) {
	var body []byte
	var err error
	if value != nil {
		body, err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
	}
	headers := map[string]string{"Authorization": "Bearer " + token}
	if value != nil {
		headers["Content-Type"] = "application/json"
	}
	raw, err := provider.NewClient(endpoint(ep)).DoReq(ctx, method, path, body, headers)
	if err != nil {
		return nil, err
	}
	var envelope response
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid Cloudflare response"}
	}
	if !envelope.Success {
		messages := make([]string, 0, len(envelope.Errors))
		for _, item := range envelope.Errors {
			messages = append(messages, item.Message)
		}
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: strings.Join(messages, "; ")}
	}
	return envelope.Result, nil
}

type account struct{ ID, Name string }

func (p *CloudflareProvider) Probe(ctx context.Context, ep string, credential []byte) (*domain.ProbeResult, error) {
	token := string(credential)
	var verify struct {
		Status string `json:"status"`
	}
	raw, err := p.request(ctx, http.MethodGet, ep, token, "/user/tokens/verify", nil)
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(raw, &verify) != nil || verify.Status != "active" {
		return nil, &provider.Error{Kind: provider.KindUnauthorized, ProviderMsg: "token is not active"}
	}
	raw, err = p.request(ctx, http.MethodGet, ep, token, "/accounts", nil)
	if err != nil {
		return nil, err
	}
	var accounts []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &accounts); err != nil {
		return nil, err
	}
	scopes := make([]domain.ProviderScope, 0, len(accounts))
	for _, a := range accounts {
		scopes = append(scopes, domain.ProviderScope{Type: "account", ID: a.ID, Label: a.Name})
	}
	permissions := map[domain.Capability]domain.CapabilityState{}
	for _, product := range provider.CloudflareDescriptor.Products {
		for _, capability := range product.Capabilities {
			permissions[capability] = domain.CapabilityState{Availability: domain.CapabilityUnknown, Reason: "Cloudflare does not expose complete token permissions"}
		}
	}
	return &domain.ProbeResult{Identity: map[string]any{"token_status": verify.Status}, Scopes: scopes, Permissions: permissions}, nil
}

func (p *CloudflareProvider) ValidateScope(ctx context.Context, ep string, credential []byte, scope domain.ProviderScope) (*domain.ProbeResult, error) {
	result, err := p.Probe(ctx, ep, credential)
	if err != nil {
		return nil, err
	}
	for _, candidate := range result.Scopes {
		if candidate.Type == scope.Type && candidate.ID == scope.ID {
			result.Scopes = []domain.ProviderScope{candidate}
			result.Identity["account_id"] = candidate.ID
			result.Identity["account_name"] = candidate.Label
			return result, nil
		}
	}
	return nil, &provider.Error{Kind: provider.KindUnauthorized, ProviderMsg: "token cannot access selected account"}
}

func accountID(conn *domain.ProviderConnection) string { return conn.Scope.ID }

func external(id, name, url string, meta map[string]any) *domain.ExternalResource {
	return &domain.ExternalResource{ExternalID: id, DisplayName: name, ExternalURL: url, Meta: meta}
}

func unsupported(kind string) error {
	return &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: fmt.Sprintf("%s is not supported", kind)}
}
