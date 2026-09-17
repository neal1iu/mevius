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

type VercelProvider struct{}

func NewProvider() *VercelProvider {
	return &VercelProvider{}
}

func (p *VercelProvider) Type() string {
	return "vercel"
}

func (p *VercelProvider) Descriptor() domain.ProviderDescriptor {
	return provider.VercelDescriptor
}

func (p *VercelProvider) ValidateCredentials(ctx context.Context, conn *domain.ProviderConnection, credential []byte) (json.RawMessage, error) {
	cl := p.client(conn)
	path := p.buildPath("/v2/user", conn)
	body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(credential))
	if err != nil {
		return nil, err
	}

	var resp struct {
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse user response: %v", err),
		}
	}

	info := map[string]any{
		"user_id":  resp.User.ID,
		"username": resp.User.Username,
	}
	if teamID, ok := p.extractTeamID(conn); ok && teamID != "" {
		info["team_id"] = teamID
	}

	raw, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("marshal account info: %w", err)
	}
	return json.RawMessage(raw), nil
}

func (p *VercelProvider) ListExternalResources(ctx context.Context, conn *domain.ProviderConnection, credential []byte, product domain.ProductType) ([]domain.ExternalResource, error) {
	cl := p.client(conn)

	switch string(product) {
	case "vercel.projects":
		path := p.buildPath("/v9/projects", conn)
		body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(credential))
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

	case "vercel.dns":
		path := p.buildPath("/v5/domains", conn)
		body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(credential))
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
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: fmt.Sprintf("vercel does not support %q product", product)}
	}
}

func (p *VercelProvider) GetResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
	cl := p.client(conn)

	path := p.buildPath("/v5/domains/"+url.PathEscape(externalID), conn)
	body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(credential))
	if err == nil {
		var domainResp struct {
			Domain *struct {
				Verified bool `json:"verified"`
			} `json:"domain"`
		}
		if json.Unmarshal(body, &domainResp) == nil && domainResp.Domain != nil {
			return &domain.ExternalResource{
				ExternalID:  externalID,
				DisplayName: externalID,
				Meta: map[string]any{
					"verified": domainResp.Domain.Verified,
				},
			}, nil
		}
	}

	fallbackPath := p.buildPath("/v9/projects/"+url.PathEscape(externalID), conn)
	body, err = cl.DoReq(ctx, "GET", fallbackPath, nil, p.authHeaders(credential))
	if err != nil {
		return nil, err
	}

	var projResp struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Framework *string `json:"framework"`
	}
	if err := json.Unmarshal(body, &projResp); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse project response: %v", err),
		}
	}

	meta := map[string]any{}
	if projResp.Framework != nil {
		meta["framework"] = *projResp.Framework
	}
	return &domain.ExternalResource{
		ExternalID:  projResp.ID,
		DisplayName: projResp.Name,
		Meta:        meta,
	}, nil
}

func (p *VercelProvider) CreateResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, spec domain.ResourceSpec) (*domain.ExternalResource, error) {
	cl := p.client(conn)
	path := p.buildPath("/v10/projects", conn)

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

	headers := p.authHeaders(credential)
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

func (p *VercelProvider) DeleteResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) error {
	cl := p.client(conn)
	path := p.buildPath("/v9/projects/"+url.PathEscape(externalID), conn)
	_, err := cl.DoReq(ctx, "DELETE", path, nil, p.authHeaders(credential))
	return err
}

func (p *VercelProvider) client(conn *domain.ProviderConnection) *provider.Client {
	baseURL := conn.Endpoint
	if baseURL == "" {
		baseURL = "https://api.vercel.com"
	}
	return provider.NewClient(baseURL)
}

func (p *VercelProvider) authHeaders(credential []byte) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + string(credential),
	}
}

func (p *VercelProvider) buildPath(basePath string, conn *domain.ProviderConnection) string {
	teamID, ok := p.extractTeamID(conn)
	if !ok || teamID == "" {
		return basePath
	}
	sep := "?"
	if strings.Contains(basePath, "?") {
		sep = "&"
	}
	return basePath + sep + "teamId=" + url.QueryEscape(teamID)
}

func (p *VercelProvider) extractTeamID(conn *domain.ProviderConnection) (string, bool) {
	if conn.Config == nil {
		return "", false
	}
	teamID, ok := conn.Config["team_id"]
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

var (
	_ provider.Provider    = (*VercelProvider)(nil)
	_ provider.Discoverer  = (*VercelProvider)(nil)
	_ provider.Inspector   = (*VercelProvider)(nil)
	_ provider.Provisioner = (*VercelProvider)(nil)
	_ provider.Deployer    = (*VercelProvider)(nil)
	_ provider.LogFetcher  = (*VercelProvider)(nil)
	_ provider.DNSManager  = (*VercelProvider)(nil)
)