package provider

import (
	"context"
	"encoding/json"

	"mevius/internal/domain"
)

type Provider interface {
	Type() string
	Descriptor() domain.ProviderDescriptor
	ValidateCredentials(ctx context.Context, conn *domain.ProviderConnection, credential []byte) (json.RawMessage, error)
}

type Inspector interface {
	GetResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error)
}

type Provisioner interface {
	CreateResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, spec domain.ResourceSpec) (*domain.ExternalResource, error)
	DeleteResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) error
}

type Deployer interface {
	TriggerDeploy(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, slot *domain.Slot) (*domain.DeployEvent, error)
	ListDeployments(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding) ([]domain.DeployEvent, error)
}

type LogFetcher interface {
	GetBuildLogs(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, deployID string, tail int) (domain.LogChunk, error)
}

type DNSManager interface {
	ListRecords(ctx context.Context, conn *domain.ProviderConnection, credential []byte, zoneID string) ([]domain.DNSRecord, error)
	CreateRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, zoneID string, record domain.DNSRecord) (*domain.DNSRecord, error)
	UpdateRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, zoneID string, recordID string, record domain.DNSRecord) (*domain.DNSRecord, error)
	DeleteRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, zoneID string, recordID string) error
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

func (r *Registry) All() map[string]Provider {
	return r.providers
}