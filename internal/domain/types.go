package domain

import (
	"context"
	"encoding/json"
	"errors"
)

type CredentialStore interface {
	Resolve(ctx context.Context, connectionID string) ([]byte, error)
}

type SyncStatus string

const (
	SyncStatusOK        SyncStatus = "ok"
	SyncStatusError     SyncStatus = "error"
	SyncStatusAuthError SyncStatus = "auth_error"
	SyncStatusOrphaned  SyncStatus = "orphaned"
	SyncStatusNever     SyncStatus = "never"
)

type LifecycleMode string

const (
	LifecycleManaged  LifecycleMode = "managed"
	LifecycleImported LifecycleMode = "imported"
)

type ResourceKind string

const (
	ResourceKindGitRepo           ResourceKind = "git_repo"
	ResourceKindCIPipeline        ResourceKind = "ci_pipeline"
	ResourceKindPage              ResourceKind = "page"
	ResourceKindServerlessService ResourceKind = "serverless_service"
	ResourceKindDNSZone           ResourceKind = "dns_zone"
)

type ProviderID string
type ProductID string
type Capability string
type AuthMethod string

const (
	AuthMethodToken AuthMethod = "token"
	AuthMethodOAuth AuthMethod = "oauth"
)

const (
	CapDiscover        Capability = "discover"
	CapInspect         Capability = "inspect"
	CapCreate          Capability = "create"
	CapDelete          Capability = "delete"
	CapDeploy          Capability = "deploy"
	CapLogs            Capability = "logs"
	CapPipelineTrigger Capability = "pipeline_trigger"
	CapPipelineCancel  Capability = "pipeline_cancel"
	CapPipelineRerun   Capability = "pipeline_rerun"
	CapPipelineRuns    Capability = "pipeline_runs"
	CapDNSManage       Capability = "dns_manage"
)

type CapabilityAvailability string

const (
	CapabilityAvailable   CapabilityAvailability = "available"
	CapabilityUnavailable CapabilityAvailability = "unavailable"
	CapabilityUnknown     CapabilityAvailability = "unknown"
)

type CapabilityState struct {
	Availability CapabilityAvailability `json:"availability"`
	Reason       string                 `json:"reason,omitempty"`
}

type ProviderDescriptor struct {
	ID          ProviderID          `json:"id"`
	DisplayName string              `json:"display_name"`
	Products    []ProductDescriptor `json:"products"`
}

type ProductDescriptor struct {
	ID           ProductID     `json:"id"`
	ProviderID   ProviderID    `json:"provider_id"`
	DisplayName  string        `json:"display_name"`
	ResourceKind ResourceKind  `json:"resource_kind"`
	Capabilities []Capability  `json:"capabilities"`
	Fields       []FieldSchema `json:"fields,omitempty"`
}

type FieldSchema struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

type ProviderScope struct {
	Type  string         `json:"type"`
	ID    string         `json:"id"`
	Label string         `json:"label"`
	Meta  map[string]any `json:"meta,omitempty"`
}

type ProbeResult struct {
	Identity    map[string]any                 `json:"identity"`
	Scopes      []ProviderScope                `json:"scopes"`
	Permissions map[Capability]CapabilityState `json:"permissions"`
}

type ProviderConnection struct {
	ID                   string                         `json:"id"`
	ProviderID           ProviderID                     `json:"provider_id"`
	Label                string                         `json:"label"`
	Endpoint             string                         `json:"endpoint,omitempty"`
	Scope                ProviderScope                  `json:"scope"`
	AuthMethod           AuthMethod                     `json:"auth_method"`
	Config               map[string]any                 `json:"config,omitempty"`
	RemoteIdentity       map[string]any                 `json:"remote_identity,omitempty"`
	Permissions          map[Capability]CapabilityState `json:"permissions,omitempty"`
	PermissionsCheckedAt string                         `json:"permissions_checked_at,omitempty"`
	CreatedAt            string                         `json:"created_at"`
	UpdatedAt            string                         `json:"updated_at"`
}

type OAuthCredential struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	Scope        string `json:"scope,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

type OAuthProviderInfo struct {
	ProviderID       ProviderID `json:"provider_id"`
	Available        bool       `json:"available"`
	Configured       bool       `json:"configured"`
	Source           string     `json:"source,omitempty"`
	ClientID         string     `json:"client_id,omitempty"`
	AuthorizationURL string     `json:"authorization_url"`
	TokenURL         string     `json:"token_url"`
	Scopes           []string   `json:"scopes"`
	PKCE             bool       `json:"pkce"`
	RedirectBaseURL  string     `json:"redirect_base_url,omitempty"`
	CallbackURL      string     `json:"callback_url,omitempty"`
	RequiresSlug     bool       `json:"requires_slug,omitempty"`
	IntegrationSlug  string     `json:"integration_slug,omitempty"`
}

type OAuthAuthorizationSession struct {
	ID          string                         `json:"id"`
	ProviderID  ProviderID                     `json:"provider_id"`
	Endpoint    string                         `json:"endpoint,omitempty"`
	Status      string                         `json:"status"`
	Identity    map[string]any                 `json:"identity,omitempty"`
	Scopes      []ProviderScope                `json:"scopes,omitempty"`
	Permissions map[Capability]CapabilityState `json:"permissions,omitempty"`
	Error       string                         `json:"error,omitempty"`
	ExpiresAt   string                         `json:"expires_at"`
}

type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type ResourceInstance struct {
	ID                string                         `json:"id"`
	ConnectionID      string                         `json:"connection_id"`
	ProviderProductID ProductID                      `json:"provider_product_id"`
	ResourceKind      ResourceKind                   `json:"resource_kind"`
	ExternalID        string                         `json:"external_id"`
	ExternalURL       string                         `json:"external_url,omitempty"`
	DisplayName       string                         `json:"display_name"`
	LifecycleMode     LifecycleMode                  `json:"lifecycle_mode"`
	Spec              json.RawMessage                `json:"spec,omitempty"`
	ProviderConfig    json.RawMessage                `json:"provider_config,omitempty"`
	CachedMeta        map[string]any                 `json:"cached_meta,omitempty"`
	SyncStatus        SyncStatus                     `json:"sync_status"`
	Capabilities      map[Capability]CapabilityState `json:"capabilities,omitempty"`
	LastSyncedAt      string                         `json:"last_synced_at,omitempty"`
	CreatedAt         string                         `json:"created_at"`
	UpdatedAt         string                         `json:"updated_at"`
}

type ProjectResource struct {
	ID                 string `json:"id"`
	ProjectID          string `json:"project_id"`
	ResourceInstanceID string `json:"resource_instance_id"`
	Alias              string `json:"alias"`
	Purpose            string `json:"purpose,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type RelationType string
type RelationOrigin string

const (
	RelationSourceRepo RelationType   = "source_repo"
	RelationDeploysTo  RelationType   = "deploys_to"
	RelationSystem     RelationOrigin = "system"
	RelationUser       RelationOrigin = "user"
)

type ResourceRelation struct {
	ID                     string          `json:"id"`
	FromResourceInstanceID string          `json:"from_resource_instance_id"`
	ToResourceInstanceID   string          `json:"to_resource_instance_id"`
	Type                   RelationType    `json:"relation_type"`
	Origin                 RelationOrigin  `json:"origin"`
	Config                 json.RawMessage `json:"config,omitempty"`
	CreatedAt              string          `json:"created_at"`
}

type ExternalResource struct {
	ExternalID     string                         `json:"external_id"`
	ExternalURL    string                         `json:"external_url,omitempty"`
	DisplayName    string                         `json:"display_name"`
	Spec           json.RawMessage                `json:"spec,omitempty"`
	ProviderConfig json.RawMessage                `json:"provider_config,omitempty"`
	Meta           map[string]any                 `json:"meta,omitempty"`
	Capabilities   map[Capability]CapabilityState `json:"capabilities,omitempty"`
}

type DiscoveryScope struct {
	Parent *ResourceInstance `json:"parent,omitempty"`
}

type CreateResourceRequest struct {
	Spec           json.RawMessage   `json:"spec"`
	ProviderConfig json.RawMessage   `json:"provider_config,omitempty"`
	Source         *ResourceInstance `json:"-"`
}

type RepoSpec struct {
	Name        string `json:"name"`
	Private     bool   `json:"private"`
	Description string `json:"description,omitempty"`
}

func (s RepoSpec) Validate() error {
	if s.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

type PageSpec struct {
	Name                 string `json:"name"`
	SourceRepoInstanceID string `json:"source_repo_instance_id"`
	ProductionBranch     string `json:"production_branch,omitempty"`
	Framework            string `json:"framework,omitempty"`
	BuildCommand         string `json:"build_command,omitempty"`
	OutputDirectory      string `json:"output_directory,omitempty"`
	RootDirectory        string `json:"root_directory,omitempty"`
}

func (s PageSpec) Validate() error {
	if s.Name == "" {
		return errors.New("name is required")
	}
	if s.SourceRepoInstanceID == "" {
		return errors.New("source_repo_instance_id is required")
	}
	return nil
}

type ExecutionStatus string

const (
	ExecutionQueued    ExecutionStatus = "queued"
	ExecutionRunning   ExecutionStatus = "running"
	ExecutionSucceeded ExecutionStatus = "succeeded"
	ExecutionFailed    ExecutionStatus = "failed"
	ExecutionCancelled ExecutionStatus = "cancelled"
	ExecutionUnknown   ExecutionStatus = "unknown"
)

type Execution struct {
	ID             string          `json:"id"`
	Status         ExecutionStatus `json:"status"`
	ProviderStatus string          `json:"provider_status,omitempty"`
	Ref            string          `json:"ref,omitempty"`
	CommitSHA      string          `json:"commit_sha,omitempty"`
	ExternalURL    string          `json:"external_url,omitempty"`
	CreatedAt      string          `json:"created_at,omitempty"`
	StartedAt      string          `json:"started_at,omitempty"`
	FinishedAt     string          `json:"finished_at,omitempty"`
}

type TriggerPipelineRequest struct {
	Ref    string            `json:"ref,omitempty"`
	Inputs map[string]string `json:"inputs,omitempty"`
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
