package vercel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

type VercelProvider struct {
	projects *projectDriver
	dns      *dnsDriver
}
type projectDriver struct{ p *VercelProvider }
type dnsDriver struct{ p *VercelProvider }

func NewProvider() *VercelProvider {
	p := &VercelProvider{}
	p.projects = &projectDriver{p}
	p.dns = &dnsDriver{p}
	return p
}
func (p *VercelProvider) ID() domain.ProviderID                 { return "vercel" }
func (p *VercelProvider) Descriptor() domain.ProviderDescriptor { return provider.VercelDescriptor }
func (p *VercelProvider) Products() []provider.ProductDriver {
	return []provider.ProductDriver{p.projects, p.dns}
}
func (d *projectDriver) Descriptor() domain.ProductDescriptor {
	return provider.ProductDescriptor(provider.VercelDescriptor, "vercel.projects")
}
func (d *dnsDriver) Descriptor() domain.ProductDescriptor {
	return provider.ProductDescriptor(provider.VercelDescriptor, "vercel.dns")
}

func endpoint(v string) string {
	if v == "" {
		return "https://api.vercel.com"
	}
	return strings.TrimRight(v, "/")
}
func scopedPath(path string, conn *domain.ProviderConnection) string {
	if conn.Scope.Type != "team" {
		return path
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "teamId=" + url.QueryEscape(conn.Scope.ID)
}
func (p *VercelProvider) request(ctx context.Context, method, ep string, credential []byte, path string, value any) ([]byte, error) {
	var body []byte
	var err error
	if value != nil {
		body, err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
	}
	headers := map[string]string{"Authorization": "Bearer " + string(credential)}
	if value != nil {
		headers["Content-Type"] = "application/json"
	}
	return provider.NewClient(endpoint(ep)).DoReq(ctx, method, path, body, headers)
}

func (p *VercelProvider) Probe(ctx context.Context, ep string, credential []byte) (*domain.ProbeResult, error) {
	raw, err := p.request(ctx, http.MethodGet, ep, credential, "/v2/user", nil)
	if err != nil {
		return nil, err
	}
	var userResp struct {
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"user"`
	}
	if err = json.Unmarshal(raw, &userResp); err != nil {
		return nil, err
	}
	scopes := []domain.ProviderScope{{Type: "personal", ID: userResp.User.ID, Label: userResp.User.Username}}
	if teamsRaw, teamErr := p.request(ctx, http.MethodGet, ep, credential, "/v2/teams", nil); teamErr == nil {
		var teamsResp struct {
			Teams []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Slug string `json:"slug"`
			} `json:"teams"`
		}
		if json.Unmarshal(teamsRaw, &teamsResp) == nil {
			for _, team := range teamsResp.Teams {
				label := team.Name
				if label == "" {
					label = team.Slug
				}
				scopes = append(scopes, domain.ProviderScope{Type: "team", ID: team.ID, Label: label})
			}
		}
	}
	permissions := map[domain.Capability]domain.CapabilityState{}
	for _, product := range provider.VercelDescriptor.Products {
		for _, capability := range product.Capabilities {
			permissions[capability] = domain.CapabilityState{Availability: domain.CapabilityUnknown, Reason: "Vercel does not expose complete token permissions"}
		}
	}
	return &domain.ProbeResult{Identity: map[string]any{"user_id": userResp.User.ID, "username": userResp.User.Username}, Scopes: scopes, Permissions: permissions}, nil
}
func (p *VercelProvider) ValidateScope(ctx context.Context, ep string, credential []byte, scope domain.ProviderScope) (*domain.ProbeResult, error) {
	result, err := p.Probe(ctx, ep, credential)
	if err != nil {
		return nil, err
	}
	for _, candidate := range result.Scopes {
		if candidate.Type == scope.Type && candidate.ID == scope.ID {
			result.Scopes = []domain.ProviderScope{candidate}
			return result, nil
		}
	}
	return nil, &provider.Error{Kind: provider.KindUnauthorized, ProviderMsg: "token cannot access selected scope"}
}

type project struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Framework *string `json:"framework"`
	Link      *struct {
		Type string `json:"type"`
		Repo string `json:"repo"`
	} `json:"link"`
}

func projectExternal(v project) *domain.ExternalResource {
	meta := map[string]any{}
	if v.Framework != nil {
		meta["framework"] = *v.Framework
	}
	if v.Link != nil {
		meta["source_type"] = v.Link.Type
		meta["source_repo"] = v.Link.Repo
	}
	return &domain.ExternalResource{ExternalID: v.ID, DisplayName: v.Name, ExternalURL: "https://vercel.com/" + v.Name, Meta: meta}
}
func (d *projectDriver) Discover(ctx context.Context, conn *domain.ProviderConnection, credential []byte, _ domain.DiscoveryScope) ([]domain.ExternalResource, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, credential, scopedPath("/v9/projects", conn), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Projects []project `json:"projects"`
	}
	if err = json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	result := make([]domain.ExternalResource, 0, len(resp.Projects))
	for _, v := range resp.Projects {
		result = append(result, *projectExternal(v))
	}
	return result, nil
}
func (d *projectDriver) Inspect(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) (*domain.ExternalResource, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, credential, scopedPath("/v9/projects/"+url.PathEscape(instance.ExternalID), conn), nil)
	if err != nil {
		return nil, err
	}
	var v project
	if err = json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return projectExternal(v), nil
}
func repoName(source *domain.ResourceInstance) (string, error) {
	if source == nil || source.ResourceKind != domain.ResourceKindGitRepo {
		return "", fmt.Errorf("Git repository source is required")
	}
	if !strings.Contains(source.ExternalID, "/") {
		return "", fmt.Errorf("repository external id must be owner/name")
	}
	return source.ExternalID, nil
}
func (d *projectDriver) Create(ctx context.Context, conn *domain.ProviderConnection, credential []byte, req domain.CreateResourceRequest) (*domain.ExternalResource, error) {
	var spec domain.PageSpec
	if err := json.Unmarshal(req.Spec, &spec); err != nil {
		return nil, err
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	repo, err := repoName(req.Source)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"name": spec.Name, "gitRepository": map[string]any{"type": "github", "repo": repo}}
	if spec.Framework != "" {
		payload["framework"] = spec.Framework
	}
	raw, err := d.p.request(ctx, http.MethodPost, conn.Endpoint, credential, scopedPath("/v10/projects", conn), payload)
	if err != nil {
		return nil, err
	}
	var v project
	if err = json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	result := projectExternal(v)
	result.Spec = req.Spec
	config, _ := json.Marshal(map[string]any{"version": 1, "source_repo": repo, "production_branch": spec.ProductionBranch})
	result.ProviderConfig = config
	return result, nil
}
func (d *projectDriver) Delete(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) error {
	_, err := d.p.request(ctx, http.MethodDelete, conn.Endpoint, credential, scopedPath("/v9/projects/"+url.PathEscape(instance.ExternalID), conn), nil)
	return err
}
func (d *projectDriver) RecoverCreate(ctx context.Context, conn *domain.ProviderConnection, credential []byte, req domain.CreateResourceRequest) (*domain.ExternalResource, error) {
	var spec domain.PageSpec
	if json.Unmarshal(req.Spec, &spec) != nil {
		return nil, nil
	}
	return d.Inspect(ctx, conn, credential, &domain.ResourceInstance{ExternalID: spec.Name})
}

type deployment struct {
	UID     string         `json:"uid"`
	URL     string         `json:"url"`
	State   string         `json:"state"`
	Created int64          `json:"created"`
	Ready   int64          `json:"ready"`
	Meta    map[string]any `json:"meta"`
}

func execution(v deployment) domain.Execution {
	return domain.Execution{ID: v.UID, Status: status(v.State), ProviderStatus: v.State, ExternalURL: "https://" + v.URL}
}
func (d *projectDriver) ListDeployments(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) ([]domain.Execution, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, credential, scopedPath("/v6/deployments?projectId="+url.QueryEscape(instance.ExternalID), conn), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Deployments []deployment `json:"deployments"`
	}
	if err = json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	result := make([]domain.Execution, 0, len(resp.Deployments))
	for _, v := range resp.Deployments {
		result = append(result, execution(v))
	}
	return result, nil
}
func (d *projectDriver) TriggerDeployment(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) (*domain.Execution, error) {
	var config struct {
		SourceRepo       string `json:"source_repo"`
		ProductionBranch string `json:"production_branch"`
	}
	_ = json.Unmarshal(instance.ProviderConfig, &config)
	if config.SourceRepo == "" {
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: "project is not Git connected"}
	}
	ref := config.ProductionBranch
	if ref == "" {
		ref = "main"
	}
	payload := map[string]any{"name": instance.DisplayName, "project": instance.ExternalID, "gitSource": map[string]any{"type": "github", "repo": config.SourceRepo, "ref": ref}}
	raw, err := d.p.request(ctx, http.MethodPost, conn.Endpoint, credential, scopedPath("/v13/deployments", conn), payload)
	if err != nil {
		return nil, err
	}
	var v deployment
	if err = json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	result := execution(v)
	return &result, nil
}
func (d *projectDriver) GetLogs(ctx context.Context, conn *domain.ProviderConnection, credential []byte, _ *domain.ResourceInstance, executionID string, tail int) (domain.LogChunk, error) {
	path := "/v3/deployments/" + url.PathEscape(executionID) + "/events"
	if tail > 0 {
		path += "?limit=" + fmt.Sprint(tail)
	}
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, credential, scopedPath(path, conn), nil)
	if err != nil {
		return domain.LogChunk{}, err
	}
	return domain.LogChunk{Lines: string(raw)}, nil
}

type domainItem struct {
	Name     string `json:"name"`
	Verified bool   `json:"verified"`
}

func (d *dnsDriver) Discover(ctx context.Context, conn *domain.ProviderConnection, credential []byte, _ domain.DiscoveryScope) ([]domain.ExternalResource, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, credential, scopedPath("/v5/domains", conn), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Domains []domainItem `json:"domains"`
	}
	if err = json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	result := make([]domain.ExternalResource, 0, len(resp.Domains))
	for _, v := range resp.Domains {
		result = append(result, domain.ExternalResource{ExternalID: v.Name, DisplayName: v.Name, Meta: map[string]any{"verified": v.Verified}})
	}
	return result, nil
}
func (d *dnsDriver) Inspect(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) (*domain.ExternalResource, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, credential, scopedPath("/v5/domains/"+url.PathEscape(instance.ExternalID), conn), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Domain domainItem `json:"domain"`
	}
	if err = json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	name := resp.Domain.Name
	if name == "" {
		name = instance.ExternalID
	}
	return &domain.ExternalResource{ExternalID: name, DisplayName: name, Meta: map[string]any{"verified": resp.Domain.Verified}}, nil
}

type dnsRecord struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"mxPriority,omitempty"`
}

func toDNS(v dnsRecord, zone string) domain.DNSRecord {
	return domain.DNSRecord{ID: v.ID, Type: v.Type, Name: v.Name, Content: v.Value, TTL: v.TTL, Priority: v.Priority, ZoneID: zone}
}
func (d *dnsDriver) ListRecords(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) ([]domain.DNSRecord, error) {
	raw, err := d.p.request(ctx, http.MethodGet, conn.Endpoint, credential, scopedPath("/v4/domains/"+url.PathEscape(instance.ExternalID)+"/records", conn), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Records []dnsRecord `json:"records"`
	}
	if err = json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	result := make([]domain.DNSRecord, 0, len(resp.Records))
	for _, v := range resp.Records {
		result = append(result, toDNS(v, instance.ExternalID))
	}
	return result, nil
}
func (d *dnsDriver) CreateRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, record domain.DNSRecord) (*domain.DNSRecord, error) {
	return d.write(ctx, http.MethodPost, conn, credential, instance, "", record)
}
func (d *dnsDriver) UpdateRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, id string, record domain.DNSRecord) (*domain.DNSRecord, error) {
	return d.write(ctx, http.MethodPatch, conn, credential, instance, id, record)
}
func (d *dnsDriver) write(ctx context.Context, method string, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, id string, record domain.DNSRecord) (*domain.DNSRecord, error) {
	path := "/v1/domains/" + url.PathEscape(instance.ExternalID) + "/records"
	if id != "" {
		path += "/" + url.PathEscape(id)
	}
	raw, err := d.p.request(ctx, method, conn.Endpoint, credential, scopedPath(path, conn), dnsRecord{Type: record.Type, Name: record.Name, Value: record.Content, TTL: record.TTL, Priority: record.Priority})
	if err != nil {
		return nil, err
	}
	var v dnsRecord
	if err = json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	result := toDNS(v, instance.ExternalID)
	return &result, nil
}
func (d *dnsDriver) DeleteRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, id string) error {
	_, err := d.p.request(ctx, http.MethodDelete, conn.Endpoint, credential, scopedPath("/v2/domains/"+url.PathEscape(instance.ExternalID)+"/records/"+url.PathEscape(id), conn), nil)
	return err
}
func status(v string) domain.ExecutionStatus {
	switch strings.ToUpper(v) {
	case "QUEUED", "INITIALIZING":
		return domain.ExecutionQueued
	case "BUILDING", "RUNNING":
		return domain.ExecutionRunning
	case "READY":
		return domain.ExecutionSucceeded
	case "ERROR":
		return domain.ExecutionFailed
	case "CANCELED", "CANCELLED":
		return domain.ExecutionCancelled
	default:
		return domain.ExecutionUnknown
	}
}
