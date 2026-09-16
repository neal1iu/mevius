package vercel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

type VercelProvider struct {
	baseURL string
}

func NewProvider(baseURL string) *VercelProvider {
	return &VercelProvider{baseURL: baseURL}
}

func (p *VercelProvider) Type() string {
	return "vercel"
}

func (p *VercelProvider) ValidateCredentials(ctx context.Context, account *domain.ProviderAccount) (domain.AccountMeta, error) {
	cl := provider.NewClient(p.baseURL)
	path := p.buildPath("/v2/user", account)
	body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(account))
	if err != nil {
		return domain.AccountMeta{}, err
	}

	var resp struct {
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return domain.AccountMeta{}, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse user response: %v", err),
		}
	}

	meta := domain.AccountMeta{
		AccountID: resp.User.ID,
		Raw: map[string]any{
			"user_id":  resp.User.ID,
			"username": resp.User.Username,
		},
	}

	if teamID, ok := p.extractTeamID(account); ok && teamID != "" {
		meta.Raw["team_id"] = teamID
	}

	return meta, nil
}

func (p *VercelProvider) ListExternalResources(ctx context.Context, account *domain.ProviderAccount, kind domain.ResourceKind) ([]domain.ExternalResource, error) {
	cl := provider.NewClient(p.baseURL)

	switch kind {
	case domain.ResourceKindStaticSite:
		path := p.buildPath("/v9/projects", account)
		body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(account))
		if err != nil {
			return nil, err
		}

		var resp struct {
			Projects []struct {
				ID        string  `json:"id"`
				Name      string  `json:"name"`
				Framework *string `json:"framework"`
			} `json:"projects"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, &provider.Error{
				Kind:        provider.KindUpstream,
				ProviderMsg: fmt.Sprintf("parse projects response: %v", err),
			}
		}

		resources := make([]domain.ExternalResource, 0, len(resp.Projects))
		for _, proj := range resp.Projects {
			meta := map[string]any{}
			if proj.Framework != nil {
				meta["framework"] = *proj.Framework
			}
			resources = append(resources, domain.ExternalResource{
				ExternalID:  proj.ID,
				DisplayName: proj.Name,
				Meta:        meta,
			})
		}
		return resources, nil

	case domain.ResourceKindDNSDomain:
		path := p.buildPath("/v5/domains", account)
		body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(account))
		if err != nil {
			return nil, err
		}

		var resp struct {
			Domains []struct {
				Name     string `json:"name"`
				Verified bool   `json:"verified"`
			} `json:"domains"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, &provider.Error{
				Kind:        provider.KindUpstream,
				ProviderMsg: fmt.Sprintf("parse domains response: %v", err),
			}
		}

		resources := make([]domain.ExternalResource, 0, len(resp.Domains))
		for _, d := range resp.Domains {
			resources = append(resources, domain.ExternalResource{
				ExternalID:  d.Name,
				DisplayName: d.Name,
				Meta: map[string]any{
					"verified": d.Verified,
				},
			})
		}
		return resources, nil

	default:
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: fmt.Sprintf("vercel does not support %q resources", kind)}
	}
}

func (p *VercelProvider) GetResource(ctx context.Context, account *domain.ProviderAccount, externalID string) (*domain.ExternalResource, error) {
	cl := provider.NewClient(p.baseURL)
	path := p.buildPath("/v9/projects/"+url.PathEscape(externalID), account)
	body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(account))
	if err != nil {
		return nil, err
	}

	var resp struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Framework *string `json:"framework"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse project response: %v", err),
		}
	}

	meta := map[string]any{}
	if resp.Framework != nil {
		meta["framework"] = *resp.Framework
	}
	return &domain.ExternalResource{
		ExternalID:  resp.ID,
		DisplayName: resp.Name,
		Meta:        meta,
	}, nil
}

func (p *VercelProvider) CreateResource(ctx context.Context, account *domain.ProviderAccount, spec domain.ResourceSpec) (*domain.ExternalResource, error) {
	cl := provider.NewClient(p.baseURL)
	path := p.buildPath("/v10/projects", account)

	payload := map[string]any{
		"name": spec.Name,
	}
	if fw := p.frameworkFromExtra(spec); fw != "" {
		payload["framework"] = fw
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal create request: %w", err)
	}

	headers := p.authHeaders(account)
	headers["Content-Type"] = "application/json"

	body, err := cl.DoReq(ctx, "POST", path, reqBody, headers)
	if err != nil {
		return nil, err
	}

	var resp struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Framework *string `json:"framework"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse create project response: %v", err),
		}
	}

	meta := map[string]any{}
	if resp.Framework != nil {
		meta["framework"] = *resp.Framework
	}
	return &domain.ExternalResource{
		ExternalID:  resp.ID,
		DisplayName: resp.Name,
		Meta:        meta,
	}, nil
}

func (p *VercelProvider) DeleteResource(ctx context.Context, account *domain.ProviderAccount, externalID string) error {
	cl := provider.NewClient(p.baseURL)
	path := p.buildPath("/v9/projects/"+url.PathEscape(externalID), account)
	_, err := cl.DoReq(ctx, "DELETE", path, nil, p.authHeaders(account))
	return err
}

func (p *VercelProvider) authHeaders(account *domain.ProviderAccount) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + account.TokenEncrypted,
	}
}

func (p *VercelProvider) buildPath(basePath string, account *domain.ProviderAccount) string {
	teamID, ok := p.extractTeamID(account)
	if !ok || teamID == "" {
		return basePath
	}
	sep := "?"
	if strings.Contains(basePath, "?") {
		sep = "&"
	}
	return basePath + sep + "teamId=" + url.QueryEscape(teamID)
}

func (p *VercelProvider) extractTeamID(account *domain.ProviderAccount) (string, bool) {
	if account.Meta.Raw == nil {
		return "", false
	}
	teamID, ok := account.Meta.Raw["team_id"]
	if !ok {
		return "", false
	}
	s, ok := teamID.(string)
	return s, ok
}

func (p *VercelProvider) frameworkFromExtra(spec domain.ResourceSpec) string {
	if spec.Extra == nil {
		return ""
	}
	fw, ok := spec.Extra["framework"]
	if !ok {
		return ""
	}
	s, ok := fw.(string)
	if !ok {
		return ""
	}
	return s
}