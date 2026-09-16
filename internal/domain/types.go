package domain

import (
	"encoding/json"
	"errors"
)

type SyncStatus string

const (
	SyncStatusOK        SyncStatus = "ok"
	SyncStatusError     SyncStatus = "error"
	SyncStatusAuthError SyncStatus = "auth_error"
	SyncStatusOrphaned  SyncStatus = "orphaned"
	SyncStatusNever     SyncStatus = "never"
)

type ResourceKind string

const (
	ResourceKindRepo       ResourceKind = "repo"
	ResourceKindCompute    ResourceKind = "compute"
	ResourceKindStaticSite ResourceKind = "static-site"
	ResourceKindDNSDomain  ResourceKind = "dns-domain"
)

type ProviderType string

const (
	ProviderTypeCloudflare ProviderType = "cloudflare"
	ProviderTypeGitHub     ProviderType = "github"
	ProviderTypeVercel     ProviderType = "vercel"
)

type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type Slot struct {
	ID        string          `json:"id"`
	ProjectID string          `json:"project_id"`
	Name      string          `json:"name"`
	Kind      ResourceKind    `json:"kind"`
	Provider  ProviderType    `json:"provider"`
	Config    json.RawMessage `json:"config,omitempty"`
	CreatedAt string          `json:"created_at"`
}

type Binding struct {
	ID           string         `json:"id"`
	SlotID       string         `json:"slot_id"`
	AccountID    string         `json:"account_id"`
	ExternalID   string         `json:"external_id"`
	ExternalURL  string         `json:"external_url,omitempty"`
	CachedMeta   map[string]any `json:"cached_meta,omitempty"`
	SyncStatus   SyncStatus     `json:"sync_status"`
	LastSyncedAt string         `json:"last_synced_at,omitempty"`
	CreatedAt    string         `json:"created_at"`
}

type ProviderAccount struct {
	ID             string       `json:"id"`
	Provider       ProviderType `json:"provider"`
	Label          string       `json:"label"`
	TokenEncrypted string       `json:"-"`
	Meta           AccountMeta  `json:"meta,omitempty"`
	CreatedAt      string       `json:"created_at"`
}

type AccountMeta struct {
	AccountID string         `json:"account_id,omitempty"`
	Raw       map[string]any `json:"raw,omitempty"`
}

type ExternalResource struct {
	ExternalID  string         `json:"external_id"`
	DisplayName string         `json:"display_name"`
	Meta        map[string]any `json:"meta,omitempty"`
}

type ResourceSpec struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Private     bool         `json:"private,omitempty"`
	Kind        ResourceKind `json:"kind"`
	Extra       map[string]any `json:"extra,omitempty"`
}

type DeployEvent struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type LogChunk struct {
	Lines     string `json:"lines"`
	Truncated bool   `json:"truncated"`
}

type DNSRecord struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl,omitempty"`
	Proxied  *bool  `json:"proxied,omitempty"`
	Priority *int   `json:"priority,omitempty"`
	ZoneID   string `json:"zone_id,omitempty"`
}

type RepoConfig struct {
	Name        string `json:"name"`
	Private     bool   `json:"private"`
	Description string `json:"description,omitempty"`
	WorkflowID  string `json:"workflow_id,omitempty"`
	WorkflowRef string `json:"workflow_ref,omitempty"`
}

func (c RepoConfig) Validate() error {
	if c.Name == "" {
		return errors.New("repo config: name is required")
	}
	return nil
}

type ComputeConfig struct {
	Name              string `json:"name"`
	CompatibilityDate string `json:"compatibility_date,omitempty"`
}

func (c ComputeConfig) Validate() error {
	if c.Name == "" {
		return errors.New("compute config: name is required")
	}
	return nil
}

type StaticSiteConfig struct {
	Name             string `json:"name"`
	Framework        string `json:"framework,omitempty"`
	BuildCommand     string `json:"build_command,omitempty"`
	OutputDir        string `json:"output_dir,omitempty"`
	RootDir          string `json:"root_dir,omitempty"`
	ProductionBranch string `json:"production_branch,omitempty"`
}

func (c StaticSiteConfig) Validate() error {
	if c.Name == "" {
		return errors.New("static site config: name is required")
	}
	return nil
}

type DnsDomainConfig struct{}

func (c DnsDomainConfig) Validate() error {
	return nil
}