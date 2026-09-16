package provider

import (
	"context"
	"mevius/internal/domain"
)

type Provider interface {
	Type() string
	ValidateCredentials(ctx context.Context, account *domain.ProviderAccount) (domain.AccountMeta, error)
	ListExternalResources(ctx context.Context, account *domain.ProviderAccount, kind domain.ResourceKind) ([]domain.ExternalResource, error)
}

type Inspector interface {
	GetResource(ctx context.Context, account *domain.ProviderAccount, externalID string) (*domain.ExternalResource, error)
}

type Provisioner interface {
	CreateResource(ctx context.Context, account *domain.ProviderAccount, spec domain.ResourceSpec) (*domain.ExternalResource, error)
	DeleteResource(ctx context.Context, account *domain.ProviderAccount, externalID string) error
}

type Deployer interface {
	TriggerDeploy(ctx context.Context, account *domain.ProviderAccount, binding *domain.Binding, slot *domain.Slot) (*domain.DeployEvent, error)
	ListDeployments(ctx context.Context, account *domain.ProviderAccount, binding *domain.Binding) ([]domain.DeployEvent, error)
}

type LogFetcher interface {
	GetBuildLogs(ctx context.Context, account *domain.ProviderAccount, binding *domain.Binding, deployID string, tail int) (domain.LogChunk, error)
}

type DNSManager interface {
	ListRecords(ctx context.Context, account *domain.ProviderAccount, zoneID string) ([]domain.DNSRecord, error)
	CreateRecord(ctx context.Context, account *domain.ProviderAccount, zoneID string, record domain.DNSRecord) (*domain.DNSRecord, error)
	UpdateRecord(ctx context.Context, account *domain.ProviderAccount, zoneID string, recordID string, record domain.DNSRecord) (*domain.DNSRecord, error)
	DeleteRecord(ctx context.Context, account *domain.ProviderAccount, zoneID string, recordID string) error
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

func (r *Registry) Register(p Provider) {
	typ := p.Type()
	if _, exists := r.providers[typ]; exists {
		panic("provider already registered: " + typ)
	}
	r.providers[typ] = p
}

func (r *Registry) Get(providerType string) Provider {
	return r.providers[providerType]
}