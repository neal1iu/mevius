package provider

import (
	"context"

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
