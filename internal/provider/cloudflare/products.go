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

type workerDriver struct{ p *CloudflareProvider }
type pageDriver struct{ p *CloudflareProvider }
type dnsDriver struct{ p *CloudflareProvider }

func (d *workerDriver) Descriptor() domain.ProductDescriptor {
	return provider.ProductDescriptor(provider.CloudflareDescriptor, "cloudflare.workers")
}
func (d *pageDriver) Descriptor() domain.ProductDescriptor {
	return provider.ProductDescriptor(provider.CloudflareDescriptor, "cloudflare.pages")
}
func (d *dnsDriver) Descriptor() domain.ProductDescriptor {
	return provider.ProductDescriptor(provider.CloudflareDescriptor, "cloudflare.dns")
}

type worker struct {
	ID         string `json:"id"`
	ModifiedOn string `json:"modified_on"`
}

func (d *workerDriver) Discover(ctx context.Context, conn *domain.ProviderConnection, credential []byte, _ domain.DiscoveryScope) ([]domain.ExternalResource, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, string(credential), "/accounts/"+accountID(conn)+"/workers/scripts", nil)
	if err != nil {
		return nil, err
	}
	var values []worker
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	result := make([]domain.ExternalResource, 0, len(values))
	for _, value := range values {
		result = append(result, *external(value.ID, value.ID, "", map[string]any{"modified_on": value.ModifiedOn}))
	}
	return result, nil
}

func (d *workerDriver) Inspect(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) (*domain.ExternalResource, error) {
	resources, err := d.Discover(ctx, conn, credential, domain.DiscoveryScope{})
	if err != nil {
		return nil, err
	}
	for _, candidate := range resources {
		if candidate.ExternalID == instance.ExternalID {
			return &candidate, nil
		}
	}
	return nil, &provider.Error{Kind: provider.KindNotFound, ProviderMsg: "worker not found"}
}

type pageProject struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CreatedOn  string `json:"created_on"`
	ModifiedOn string `json:"modified_on"`
	Subdomain  string `json:"subdomain"`
	Source     *struct {
		Type   string `json:"type"`
		Config *struct {
			Owner            string `json:"owner"`
			RepoName         string `json:"repo_name"`
			ProductionBranch string `json:"production_branch"`
		} `json:"config"`
	} `json:"source"`
}

func pageExternal(value pageProject) *domain.ExternalResource {
	meta := map[string]any{"created_on": value.CreatedOn, "modified_on": value.ModifiedOn}
	capabilities := map[domain.Capability]domain.CapabilityState{}
	if value.Source == nil || value.Source.Type != "github" {
		capabilities[domain.CapDeploy] = domain.CapabilityState{Availability: domain.CapabilityUnavailable, Reason: "Direct Upload Pages projects cannot be deployed through this integration"}
	} else if value.Source.Config != nil {
		meta["source_repo"] = value.Source.Config.Owner + "/" + value.Source.Config.RepoName
		meta["production_branch"] = value.Source.Config.ProductionBranch
	}
	return &domain.ExternalResource{ExternalID: value.Name, DisplayName: value.Name, ExternalURL: value.Subdomain, Meta: meta, Capabilities: capabilities}
}

func (d *pageDriver) Discover(ctx context.Context, conn *domain.ProviderConnection, credential []byte, _ domain.DiscoveryScope) ([]domain.ExternalResource, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, string(credential), "/accounts/"+accountID(conn)+"/pages/projects", nil)
	if err != nil {
		return nil, err
	}
	var values []pageProject
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	result := make([]domain.ExternalResource, 0, len(values))
	for _, value := range values {
		result = append(result, *pageExternal(value))
	}
	return result, nil
}

func (d *pageDriver) Inspect(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) (*domain.ExternalResource, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, string(credential), "/accounts/"+accountID(conn)+"/pages/projects/"+instance.ExternalID, nil)
	if err != nil {
		return nil, err
	}
	var value pageProject
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return pageExternal(value), nil
}

func repoCoordinates(source *domain.ResourceInstance) (string, string, error) {
	if source == nil || source.ResourceKind != domain.ResourceKindGitRepo {
		return "", "", fmt.Errorf("Git repository source is required")
	}
	parts := strings.SplitN(source.ExternalID, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("repository external id must be owner/name")
	}
	return parts[0], parts[1], nil
}

func (d *pageDriver) Create(ctx context.Context, conn *domain.ProviderConnection, credential []byte, req domain.CreateResourceRequest) (*domain.ExternalResource, error) {
	var spec domain.PageSpec
	if err := json.Unmarshal(req.Spec, &spec); err != nil {
		return nil, err
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	owner, repo, err := repoCoordinates(req.Source)
	if err != nil {
		return nil, err
	}
	if spec.ProductionBranch == "" {
		spec.ProductionBranch = "main"
	}
	payload := map[string]any{"name": spec.Name, "production_branch": spec.ProductionBranch, "source": map[string]any{"type": "github", "config": map[string]any{"owner": owner, "repo_name": repo, "production_branch": spec.ProductionBranch, "pr_comments_enabled": true, "deployments_enabled": true}}}
	raw, err := d.p.request(ctx, http.MethodPost, conn.Endpoint, string(credential), "/accounts/"+accountID(conn)+"/pages/projects", payload)
	if err != nil {
		return nil, err
	}
	var value pageProject
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	result := pageExternal(value)
	result.Spec = req.Spec
	result.ProviderConfig = mustJSON(map[string]any{"version": 1, "source_repo": owner + "/" + repo, "production_branch": spec.ProductionBranch})
	return result, nil
}

func (d *pageDriver) Delete(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) error {
	_, err := d.p.request(ctx, http.MethodDelete, conn.Endpoint, string(credential), "/accounts/"+accountID(conn)+"/pages/projects/"+instance.ExternalID, nil)
	return err
}
func (d *pageDriver) RecoverCreate(ctx context.Context, conn *domain.ProviderConnection, credential []byte, req domain.CreateResourceRequest) (*domain.ExternalResource, error) {
	var spec domain.PageSpec
	if json.Unmarshal(req.Spec, &spec) != nil {
		return nil, nil
	}
	return d.Inspect(ctx, conn, credential, &domain.ResourceInstance{ExternalID: spec.Name})
}

type deployment struct {
	ID         string           `json:"id"`
	Status     string           `json:"status"`
	CreatedOn  string           `json:"created_on"`
	ModifiedOn string           `json:"modified_on"`
	URL        string           `json:"url"`
	Stages     []map[string]any `json:"stages"`
}

func execution(value deployment) domain.Execution {
	return domain.Execution{ID: value.ID, Status: mapStatus(value.Status), ProviderStatus: value.Status, ExternalURL: value.URL, CreatedAt: value.CreatedOn, FinishedAt: value.ModifiedOn}
}

func (d *pageDriver) ListDeployments(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) ([]domain.Execution, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, string(credential), "/accounts/"+accountID(conn)+"/pages/projects/"+instance.ExternalID+"/deployments", nil)
	if err != nil {
		return nil, err
	}
	var values []deployment
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	result := make([]domain.Execution, 0, len(values))
	for _, value := range values {
		result = append(result, execution(value))
	}
	return result, nil
}

func (d *pageDriver) TriggerDeployment(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) (*domain.Execution, error) {
	values, err := d.ListDeployments(ctx, conn, credential, instance)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, unsupported("deployment before the first Git push")
	}
	raw, err := d.p.request(ctx, http.MethodPost, conn.Endpoint, string(credential), "/accounts/"+accountID(conn)+"/pages/projects/"+instance.ExternalID+"/deployments/"+values[0].ID+"/retry", nil)
	if err != nil {
		return nil, err
	}
	var value deployment
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	result := execution(value)
	return &result, nil
}

func (d *pageDriver) GetLogs(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, executionID string, _ int) (domain.LogChunk, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, string(credential), "/accounts/"+accountID(conn)+"/pages/projects/"+instance.ExternalID+"/deployments/"+executionID, nil)
	if err != nil {
		return domain.LogChunk{}, err
	}
	var value deployment
	if err := json.Unmarshal(raw, &value); err != nil {
		return domain.LogChunk{}, err
	}
	lines, _ := json.MarshalIndent(value.Stages, "", "  ")
	return domain.LogChunk{Lines: string(lines), Meta: map[string]any{"note": "Cloudflare API exposes stage metadata, not raw build output"}}, nil
}

type zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (d *dnsDriver) Discover(ctx context.Context, conn *domain.ProviderConnection, credential []byte, _ domain.DiscoveryScope) ([]domain.ExternalResource, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, string(credential), "/zones?account.id="+accountID(conn), nil)
	if err != nil {
		return nil, err
	}
	var values []zone
	if err = json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	result := make([]domain.ExternalResource, 0, len(values))
	for _, v := range values {
		result = append(result, *external(v.ID, v.Name, "", map[string]any{"status": v.Status}))
	}
	return result, nil
}
func (d *dnsDriver) Inspect(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) (*domain.ExternalResource, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, string(credential), "/zones/"+instance.ExternalID, nil)
	if err != nil {
		return nil, err
	}
	var v zone
	if err = json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return external(v.ID, v.Name, "", map[string]any{"status": v.Status}), nil
}

type dnsRecord struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl,omitempty"`
	Proxied  *bool  `json:"proxied,omitempty"`
	Priority *int   `json:"priority,omitempty"`
}

func toDNS(v dnsRecord, zone string) domain.DNSRecord {
	return domain.DNSRecord{ID: v.ID, Type: v.Type, Name: v.Name, Content: v.Content, TTL: v.TTL, Proxied: v.Proxied, Priority: v.Priority, ZoneID: zone}
}
func (d *dnsDriver) ListRecords(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) ([]domain.DNSRecord, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, string(credential), "/zones/"+instance.ExternalID+"/dns_records", nil)
	if err != nil {
		return nil, err
	}
	var values []dnsRecord
	if err = json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	result := make([]domain.DNSRecord, 0, len(values))
	for _, v := range values {
		result = append(result, toDNS(v, instance.ExternalID))
	}
	return result, nil
}
func (d *dnsDriver) CreateRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, record domain.DNSRecord) (*domain.DNSRecord, error) {
	return d.writeRecord(ctx, http.MethodPost, conn, credential, instance, "", record)
}
func (d *dnsDriver) UpdateRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, id string, record domain.DNSRecord) (*domain.DNSRecord, error) {
	return d.writeRecord(ctx, http.MethodPatch, conn, credential, instance, id, record)
}
func (d *dnsDriver) writeRecord(ctx context.Context, method string, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, id string, record domain.DNSRecord) (*domain.DNSRecord, error) {
	path := "/zones/" + instance.ExternalID + "/dns_records"
	if id != "" {
		path += "/" + id
	}
	raw, err := d.p.request(ctx, method, conn.Endpoint, string(credential), path, dnsRecord{Type: record.Type, Name: record.Name, Content: record.Content, TTL: record.TTL, Proxied: record.Proxied, Priority: record.Priority})
	if err != nil {
		return nil, err
	}
	var value dnsRecord
	if err = json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	result := toDNS(value, instance.ExternalID)
	return &result, nil
}
func (d *dnsDriver) DeleteRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, id string) error {
	_, err := d.p.request(ctx, http.MethodDelete, conn.Endpoint, string(credential), "/zones/"+instance.ExternalID+"/dns_records/"+id, nil)
	return err
}

func mustJSON(v any) json.RawMessage { raw, _ := json.Marshal(v); return raw }
func mapStatus(v string) domain.ExecutionStatus {
	switch strings.ToLower(v) {
	case "queued", "initializing":
		return domain.ExecutionQueued
	case "running", "in_progress":
		return domain.ExecutionRunning
	case "success", "succeeded":
		return domain.ExecutionSucceeded
	case "failure", "failed":
		return domain.ExecutionFailed
	case "cancelled", "canceled":
		return domain.ExecutionCancelled
	default:
		return domain.ExecutionUnknown
	}
}
