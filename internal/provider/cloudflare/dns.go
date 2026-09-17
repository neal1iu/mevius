package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

type cfZone struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	NameServers []string `json:"name_servers"`
}

type cfDNSRecord struct {
	ID         string `json:"id,omitempty"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	TTL        int    `json:"ttl"`
	Proxied    *bool  `json:"proxied,omitempty"`
	Priority   *int   `json:"priority,omitempty"`
	ZoneID     string `json:"zone_id,omitempty"`
	ZoneName   string `json:"zone_name,omitempty"`
	CreatedOn  string `json:"created_on,omitempty"`
	ModifiedOn string `json:"modified_on,omitempty"`
}

func (p *CloudflareProvider) listZones(ctx context.Context, token, endpoint string) ([]cfZone, error) {
	body, err := p.doGet(ctx, token, endpoint, "/zones")
	if err != nil {
		return nil, err
	}
	resp, err := parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse zones list: %v", err),
		}
	}
	if !resp.Success {
		msg := errorMsg(resp.Errors)
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: msg,
		}
	}
	var zones []cfZone
	if err := json.Unmarshal(resp.Result, &zones); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "parse zones list result",
		}
	}
	return zones, nil
}

func (p *CloudflareProvider) getZone(ctx context.Context, token, endpoint, zoneID string) (*cfZone, error) {
	body, err := p.doGet(ctx, token, endpoint, "/zones/"+zoneID)
	if err != nil {
		return nil, err
	}
	resp, err := parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse zone detail: %v", err),
		}
	}
	if !resp.Success {
		msg := errorMsg(resp.Errors)
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: msg,
		}
	}
	var zone cfZone
	if err := json.Unmarshal(resp.Result, &zone); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "parse zone detail result",
		}
	}
	return &zone, nil
}

func (p *CloudflareProvider) listDNSRecords(ctx context.Context, token, endpoint, zoneID string) ([]cfDNSRecord, error) {
	body, err := p.doGet(ctx, token, endpoint, "/zones/"+zoneID+"/dns_records")
	if err != nil {
		return nil, err
	}
	resp, err := parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse dns records list: %v", err),
		}
	}
	if !resp.Success {
		msg := errorMsg(resp.Errors)
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: msg,
		}
	}
	var records []cfDNSRecord
	if err := json.Unmarshal(resp.Result, &records); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "parse dns records list result",
		}
	}
	return records, nil
}

func (p *CloudflareProvider) createDNSRecord(ctx context.Context, token, endpoint, zoneID string, record cfDNSRecord) (*cfDNSRecord, error) {
	bodyBytes, err := json.Marshal(record)
	if err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("marshal dns record: %v", err),
		}
	}
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}
	body, err := p.cfClient(endpoint).DoReq(ctx, "POST", "/zones/"+zoneID+"/dns_records", bodyBytes, headers)
	if err != nil {
		return nil, err
	}
	resp, err := parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse create dns record: %v", err),
		}
	}
	if !resp.Success {
		msg := errorMsg(resp.Errors)
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: msg,
		}
	}
	var created cfDNSRecord
	if err := json.Unmarshal(resp.Result, &created); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "parse create dns record result",
		}
	}
	return &created, nil
}

func (p *CloudflareProvider) updateDNSRecord(ctx context.Context, token, endpoint, zoneID, recordID string, record cfDNSRecord) (*cfDNSRecord, error) {
	bodyBytes, err := json.Marshal(record)
	if err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("marshal dns record: %v", err),
		}
	}
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}
	body, err := p.cfClient(endpoint).DoReq(ctx, "PATCH", "/zones/"+zoneID+"/dns_records/"+recordID, bodyBytes, headers)
	if err != nil {
		return nil, err
	}
	resp, err := parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse update dns record: %v", err),
		}
	}
	if !resp.Success {
		msg := errorMsg(resp.Errors)
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: msg,
		}
	}
	var updated cfDNSRecord
	if err := json.Unmarshal(resp.Result, &updated); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "parse update dns record result",
		}
	}
	return &updated, nil
}

func (p *CloudflareProvider) deleteDNSRecord(ctx context.Context, token, endpoint, zoneID, recordID string) error {
	_, err := p.doDelete(ctx, token, endpoint, "/zones/"+zoneID+"/dns_records/"+recordID)
	return err
}

func (p *CloudflareProvider) ListRecords(ctx context.Context, conn *domain.ProviderConnection, credential []byte, zoneID string) ([]domain.DNSRecord, error) {
	token := extractToken(credential)
	endpoint := defaultEndpoint(conn.Endpoint)

	records, err := p.listDNSRecords(ctx, token, endpoint, zoneID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.DNSRecord, len(records))
	for i, r := range records {
		rec := domain.DNSRecord{
			ID:       r.ID,
			Type:     r.Type,
			Name:     r.Name,
			Content:  r.Content,
			TTL:      r.TTL,
			Proxied:  r.Proxied,
			Priority: r.Priority,
			ZoneID:   zoneID,
		}
		result[i] = rec
	}
	return result, nil
}

func (p *CloudflareProvider) CreateRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, zoneID string, record domain.DNSRecord) (*domain.DNSRecord, error) {
	token := extractToken(credential)
	endpoint := defaultEndpoint(conn.Endpoint)

	cfRecord := cfDNSRecord{
		Type:     record.Type,
		Name:     record.Name,
		Content:  record.Content,
		TTL:      record.TTL,
		Proxied:  record.Proxied,
		Priority: record.Priority,
	}
	created, err := p.createDNSRecord(ctx, token, endpoint, zoneID, cfRecord)
	if err != nil {
		return nil, err
	}
	return &domain.DNSRecord{
		ID:       created.ID,
		Type:     created.Type,
		Name:     created.Name,
		Content:  created.Content,
		TTL:      created.TTL,
		Proxied:  created.Proxied,
		Priority: created.Priority,
		ZoneID:   zoneID,
	}, nil
}

func (p *CloudflareProvider) UpdateRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, zoneID string, recordID string, record domain.DNSRecord) (*domain.DNSRecord, error) {
	token := extractToken(credential)
	endpoint := defaultEndpoint(conn.Endpoint)

	cfRecord := cfDNSRecord{
		Type:     record.Type,
		Name:     record.Name,
		Content:  record.Content,
		TTL:      record.TTL,
		Proxied:  record.Proxied,
		Priority: record.Priority,
	}
	updated, err := p.updateDNSRecord(ctx, token, endpoint, zoneID, recordID, cfRecord)
	if err != nil {
		return nil, err
	}
	return &domain.DNSRecord{
		ID:       updated.ID,
		Type:     updated.Type,
		Name:     updated.Name,
		Content:  updated.Content,
		TTL:      updated.TTL,
		Proxied:  updated.Proxied,
		Priority: updated.Priority,
		ZoneID:   zoneID,
	}, nil
}

func (p *CloudflareProvider) DeleteRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, zoneID string, recordID string) error {
	token := extractToken(credential)
	endpoint := defaultEndpoint(conn.Endpoint)

	return p.deleteDNSRecord(ctx, token, endpoint, zoneID, recordID)
}