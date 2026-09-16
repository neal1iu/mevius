package vercel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

func (p *VercelProvider) ListRecords(ctx context.Context, account *domain.ProviderAccount, zoneID string) ([]domain.DNSRecord, error) {
	cl := provider.NewClient(p.baseURL)
	path := p.buildPath("/v5/domains/"+url.PathEscape(zoneID)+"/records", account)
	body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(account))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Records []struct {
			ID        string `json:"id"`
			Slug      string `json:"slug"`
			Name      string `json:"name"`
			Type      string `json:"type"`
			Value     string `json:"value"`
			Creator   string `json:"creator"`
			Domain    string `json:"domain"`
			TTL       int    `json:"ttl"`
			CreatedAt string `json:"createdAt"`
			UpdatedAt string `json:"updatedAt"`
		} `json:"records"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse records list: %v", err),
		}
	}

	records := make([]domain.DNSRecord, 0, len(resp.Records))
	for _, r := range resp.Records {
		records = append(records, domain.DNSRecord{
			ID:      r.ID,
			Type:    r.Type,
			Name:    r.Name,
			Content: r.Value,
			TTL:     r.TTL,
			ZoneID:  zoneID,
		})
	}
	return records, nil
}

func (p *VercelProvider) CreateRecord(ctx context.Context, account *domain.ProviderAccount, zoneID string, record domain.DNSRecord) (*domain.DNSRecord, error) {
	cl := provider.NewClient(p.baseURL)
	path := p.buildPath("/v2/domains/"+url.PathEscape(zoneID)+"/records", account)

	payload := map[string]any{
		"type":  record.Type,
		"name":  record.Name,
		"value": record.Content,
	}
	if record.TTL > 0 {
		payload["ttl"] = record.TTL
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal create record: %w", err)
	}

	headers := p.authHeaders(account)
	headers["Content-Type"] = "application/json"

	body, err := cl.DoReq(ctx, "POST", path, reqBody, headers)
	if err != nil {
		return nil, err
	}

	var resp struct {
		UID     string `json:"uid"`
		Updated int64  `json:"updated"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse create record response: %v", err),
		}
	}

	result := record
	result.ID = resp.UID
	result.ZoneID = zoneID
	return &result, nil
}

func (p *VercelProvider) UpdateRecord(ctx context.Context, account *domain.ProviderAccount, zoneID string, recordID string, record domain.DNSRecord) (*domain.DNSRecord, error) {
	cl := provider.NewClient(p.baseURL)
	path := p.buildPath("/v1/domains/records/"+url.PathEscape(recordID), account)

	payload := map[string]any{}
	if record.Type != "" {
		payload["type"] = record.Type
	}
	if record.Name != "" {
		payload["name"] = record.Name
	}
	if record.Content != "" {
		payload["value"] = record.Content
	}
	if record.TTL > 0 {
		payload["ttl"] = record.TTL
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal update record: %w", err)
	}

	headers := p.authHeaders(account)
	headers["Content-Type"] = "application/json"

	body, err := cl.DoReq(ctx, "PATCH", path, reqBody, headers)
	if err != nil {
		return nil, err
	}

	var resp struct {
		ID        string `json:"id"`
		Slug      string `json:"slug"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		Value     string `json:"value"`
		Creator   string `json:"creator"`
		Domain    string `json:"domain"`
		TTL       int    `json:"ttl"`
		CreatedAt int64  `json:"createdAt"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse update record response: %v", err),
		}
	}

	return &domain.DNSRecord{
		ID:      resp.ID,
		Type:    resp.Type,
		Name:    resp.Name,
		Content: resp.Value,
		TTL:     resp.TTL,
		ZoneID:  zoneID,
	}, nil
}

func (p *VercelProvider) DeleteRecord(ctx context.Context, account *domain.ProviderAccount, zoneID string, recordID string) error {
	cl := provider.NewClient(p.baseURL)
	path := p.buildPath("/v2/domains/"+url.PathEscape(zoneID)+"/records/"+url.PathEscape(recordID), account)
	_, err := cl.DoReq(ctx, "DELETE", path, nil, p.authHeaders(account))
	return err
}