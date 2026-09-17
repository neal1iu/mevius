package domain

import (
	"context"
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
	ResourceKindRepository ResourceKind = "repository"
	ResourceKindWorker     ResourceKind = "worker"
	ResourceKindStaticSite ResourceKind = "static_site"
	ResourceKindDNSZone    ResourceKind = "dns_zone"
)

type SlotRole string

const (
	SlotRoleSource   SlotRole = "source"
	SlotRoleFrontend SlotRole = "frontend"
	SlotRoleBackend  SlotRole = "backend"
	SlotRoleDatabase SlotRole = "database"
	SlotRoleDNS      SlotRole = "dns"
)

type ProviderType string

const (
	ProviderTypeCloudflare ProviderType = "cloudflare"
	ProviderTypeGitHub     ProviderType = "github"
	ProviderTypeVercel     ProviderType = "vercel"
)

type ProductType string

type Capability string

const (
	CapDiscover   Capability = "discover"
	CapInspect    Capability = "inspect"
	CapCreate     Capability = "create"
	CapDelete     Capability = "delete"
	CapWorkflow   Capability = "workflow"
	CapDeploy     Capability = "deploy"
	CapLogs       Capability = "logs"
	CapDNSManage  Capability = "dns_manage"
)

type ProviderDescriptor struct {
	Type         ProviderType         `json:"type"`
	DisplayName  string               `json:"display_name"`
	Products     []ProductDescriptor  `json:"products"`
}

type ProductDescriptor struct {
	ID           string       `json:"id"`
	DisplayName  string       `json:"display_name,omitempty"`
	ResourceKind ResourceKind `json:"resource_kind"`
	Roles        []SlotRole   `json:"roles"`
	Capabilities []Capability `json:"capabilities"`
	ConfigSchema map[string]any `json:"config_schema,omitempty"`
}

type CredentialStore interface {
	Resolve(ctx context.Context, ref string) ([]byte, error)
}

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
	Role      SlotRole        `json:"role"`
	Config    json.RawMessage `json:"config,omitempty"`
	CreatedAt string          `json:"created_at"`
}

type Binding struct {
	ID           string         `json:"id"`
	SlotID       string         `json:"slot_id"`
	ConnectionID string         `json:"connection_id"`
	ExternalID   string         `json:"external_id"`
	ExternalURL  string         `json:"external_url,omitempty"`
	Product      ProductType    `json:"product"`
	CachedMeta   map[string]any `json:"cached_meta,omitempty"`
	SyncStatus   SyncStatus     `json:"sync_status"`
	LastSyncedAt string         `json:"last_synced_at,omitempty"`
	CreatedAt    string         `json:"created_at"`
}

type ProviderConnection struct {
	ID             string         `json:"id"`
	Provider       ProviderType   `json:"provider"`
	Label          string         `json:"label"`
	Endpoint       string         `json:"endpoint,omitempty"`
	Config         map[string]any `json:"config,omitempty"`
	CredentialRef  string         `json:"-"`
	RemoteIdentity AccountMeta    `json:"remote_identity,omitempty"`
	CreatedAt      string         `json:"created_at"`
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
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Private     bool           `json:"private,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
}

type DeployEvent struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type LogChunk struct {
	Lines     string         `json:"lines"`
	Truncated bool           `json:"truncated"`
	Meta      map[string]any `json:"meta,omitempty"`
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