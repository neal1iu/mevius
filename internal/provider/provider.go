package provider

import (
	"context"
	"fmt"

	"mevius/internal/domain"
)

type Provider interface {
	ID() domain.ProviderID
	Descriptor() domain.ProviderDescriptor
	Probe(ctx context.Context, endpoint string, credential []byte) (*domain.ProbeResult, error)
	ValidateScope(ctx context.Context, endpoint string, credential []byte, scope domain.ProviderScope) (*domain.ProbeResult, error)
	Products() []ProductDriver
}

type ProductDriver interface {
	Descriptor() domain.ProductDescriptor
}

type Discoverer interface {
	Discover(ctx context.Context, conn *domain.ProviderConnection, credential []byte, scope domain.DiscoveryScope) ([]domain.ExternalResource, error)
}

type Inspector interface {
	Inspect(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) (*domain.ExternalResource, error)
}

type Provisioner interface {
	Create(ctx context.Context, conn *domain.ProviderConnection, credential []byte, req domain.CreateResourceRequest) (*domain.ExternalResource, error)
	Delete(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) error
}

type CreateRecovery interface {
	RecoverCreate(ctx context.Context, conn *domain.ProviderConnection, credential []byte, req domain.CreateResourceRequest) (*domain.ExternalResource, error)
}

type Deployer interface {
	TriggerDeployment(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) (*domain.Deployment, error)
	ListDeployments(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) ([]domain.Deployment, error)
}

type PipelineRunner interface {
	TriggerPipeline(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, req domain.TriggerPipelineRequest) (*domain.Execution, error)
	ListPipelineRuns(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) ([]domain.Execution, error)
	GetPipelineRun(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, runID string) (*domain.Execution, error)
	CancelPipelineRun(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, runID string) error
	RerunPipeline(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, runID string) (*domain.Execution, error)
}

type PipelineLogSource interface {
	GetPipelineLogs(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, executionID string, tail int) (domain.LogChunk, error)
}

type DeploymentLogSource interface {
	GetDeploymentLogs(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, deploymentID string, tail int) (domain.LogChunk, error)
}

type DNSManager interface {
	ListRecords(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) ([]domain.DNSRecord, error)
	CreateRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, record domain.DNSRecord) (*domain.DNSRecord, error)
	UpdateRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, recordID string, record domain.DNSRecord) (*domain.DNSRecord, error)
	DeleteRecord(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, recordID string) error
}

type Registry struct {
	providers map[domain.ProviderID]Provider
	products  map[domain.ProductID]ProductDriver
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[domain.ProviderID]Provider),
		products:  make(map[domain.ProductID]ProductDriver),
	}
}

func (r *Registry) Register(p Provider) {
	id := p.ID()
	if descriptorID := p.Descriptor().ID; descriptorID != id {
		panic(fmt.Sprintf("provider descriptor %s registered as %s", descriptorID, id))
	}
	if _, exists := r.providers[id]; exists {
		panic("provider already registered: " + string(id))
	}
	r.providers[id] = p
	for _, product := range p.Products() {
		d := product.Descriptor()
		if d.ProviderID != id {
			panic(fmt.Sprintf("product %s belongs to %s, registered by %s", d.ID, d.ProviderID, id))
		}
		if _, exists := r.products[d.ID]; exists {
			panic("product already registered: " + string(d.ID))
		}
		if len(d.CompatibleRoles) == 0 {
			panic("product has no compatible roles: " + string(d.ID))
		}
		roles := make(map[domain.ResourceRole]struct{}, len(d.CompatibleRoles))
		for _, role := range d.CompatibleRoles {
			if !domain.IsResourceRole(role) {
				panic("product has unknown compatible role: " + string(d.ID) + ": " + string(role))
			}
			if _, exists := roles[role]; exists {
				panic("product has duplicate compatible role: " + string(d.ID) + ": " + string(role))
			}
			roles[role] = struct{}{}
		}
		r.products[d.ID] = product
	}
}

func (r *Registry) Provider(id domain.ProviderID) Provider    { return r.providers[id] }
func (r *Registry) Product(id domain.ProductID) ProductDriver { return r.products[id] }

func (r *Registry) Resolve(id domain.ProductID) (Provider, ProductDriver, error) {
	product := r.products[id]
	if product == nil {
		return nil, nil, fmt.Errorf("product %s not registered", id)
	}
	p := r.providers[product.Descriptor().ProviderID]
	if p == nil {
		return nil, nil, fmt.Errorf("provider %s not registered", product.Descriptor().ProviderID)
	}
	return p, product, nil
}

func (r *Registry) Descriptors() []domain.ProviderDescriptor {
	result := make([]domain.ProviderDescriptor, 0, len(r.providers))
	for _, p := range r.providers {
		descriptor := p.Descriptor()
		products := p.Products()
		descriptor.Products = make([]domain.ProductDescriptor, 0, len(products))
		for _, product := range products {
			descriptor.Products = append(descriptor.Products, product.Descriptor())
		}
		result = append(result, descriptor)
	}
	return result
}
