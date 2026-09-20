package service

import (
	"context"
	"fmt"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

type RuntimeService struct{ resources *ResourceService }

func NewRuntimeService(resources *ResourceService) *RuntimeService {
	return &RuntimeService{resources: resources}
}
func (s *RuntimeService) driver(ctx context.Context, id string) (domain.ResourceInstance, domain.ProviderConnection, []byte, provider.ProductDriver, error) {
	instance, err := s.resources.Get(ctx, id, false)
	if err != nil {
		return instance, domain.ProviderConnection{}, nil, nil, err
	}
	conn, credential, driver, err := s.resources.resolve(ctx, instance.ConnectionID, instance.ProviderProductID)
	return instance, conn, credential, driver, err
}
func (s *RuntimeService) TriggerDeployment(ctx context.Context, id string) (*domain.Deployment, error) {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return nil, err
	}
	value, ok := driver.(provider.Deployer)
	if !ok {
		return nil, ErrUnsupported
	}
	result, err := value.TriggerDeployment(ctx, &conn, credential, &instance)
	if result != nil {
		result.ResourceInstanceID = instance.ID
	}
	return result, err
}
func (s *RuntimeService) ListDeployments(ctx context.Context, id string) ([]domain.Deployment, error) {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return nil, err
	}
	value, ok := driver.(provider.Deployer)
	if !ok {
		return nil, ErrUnsupported
	}
	result, err := value.ListDeployments(ctx, &conn, credential, &instance)
	for i := range result {
		result[i].ResourceInstanceID = instance.ID
	}
	return result, err
}
func (s *RuntimeService) TriggerPipeline(ctx context.Context, id string, req domain.TriggerPipelineRequest) (*domain.Execution, error) {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return nil, err
	}
	value, ok := driver.(provider.PipelineRunner)
	if !ok {
		return nil, ErrUnsupported
	}
	result, err := value.TriggerPipeline(ctx, &conn, credential, &instance, req)
	if result != nil {
		result.ResourceInstanceID = instance.ID
	}
	return result, err
}
func (s *RuntimeService) ListPipelineRuns(ctx context.Context, id string) ([]domain.Execution, error) {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return nil, err
	}
	value, ok := driver.(provider.PipelineRunner)
	if !ok {
		return nil, ErrUnsupported
	}
	result, err := value.ListPipelineRuns(ctx, &conn, credential, &instance)
	for i := range result {
		result[i].ResourceInstanceID = instance.ID
	}
	return result, err
}
func (s *RuntimeService) GetPipelineRun(ctx context.Context, id, runID string) (*domain.Execution, error) {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return nil, err
	}
	value, ok := driver.(provider.PipelineRunner)
	if !ok {
		return nil, ErrUnsupported
	}
	result, err := value.GetPipelineRun(ctx, &conn, credential, &instance, runID)
	if result != nil {
		result.ResourceInstanceID = instance.ID
	}
	return result, err
}
func (s *RuntimeService) PipelineAction(ctx context.Context, id, runID, action string) (*domain.Execution, error) {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return nil, err
	}
	value, ok := driver.(provider.PipelineRunner)
	if !ok {
		return nil, ErrUnsupported
	}
	switch action {
	case "cancel":
		return nil, value.CancelPipelineRun(ctx, &conn, credential, &instance, runID)
	case "rerun":
		result, err := value.RerunPipeline(ctx, &conn, credential, &instance, runID)
		if result != nil {
			result.ResourceInstanceID = instance.ID
		}
		return result, err
	default:
		return nil, fmt.Errorf("%w: unknown pipeline action", ErrInvalid)
	}
}
func (s *RuntimeService) PipelineLogs(ctx context.Context, id, executionID string, tail int) (domain.LogChunk, error) {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return domain.LogChunk{}, err
	}
	value, ok := driver.(provider.PipelineLogSource)
	if !ok {
		return domain.LogChunk{}, ErrUnsupported
	}
	return value.GetPipelineLogs(ctx, &conn, credential, &instance, executionID, tail)
}
func (s *RuntimeService) DeploymentLogs(ctx context.Context, id, deploymentID string, tail int) (domain.LogChunk, error) {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return domain.LogChunk{}, err
	}
	value, ok := driver.(provider.DeploymentLogSource)
	if !ok {
		return domain.LogChunk{}, ErrUnsupported
	}
	return value.GetDeploymentLogs(ctx, &conn, credential, &instance, deploymentID, tail)
}
func (s *RuntimeService) ListDNS(ctx context.Context, id string) ([]domain.DNSRecord, error) {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return nil, err
	}
	value, ok := driver.(provider.DNSManager)
	if !ok {
		return nil, ErrUnsupported
	}
	return value.ListRecords(ctx, &conn, credential, &instance)
}
func (s *RuntimeService) CreateDNS(ctx context.Context, id string, record domain.DNSRecord) (*domain.DNSRecord, error) {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return nil, err
	}
	value, ok := driver.(provider.DNSManager)
	if !ok {
		return nil, ErrUnsupported
	}
	return value.CreateRecord(ctx, &conn, credential, &instance, record)
}
func (s *RuntimeService) UpdateDNS(ctx context.Context, id, recordID string, record domain.DNSRecord) (*domain.DNSRecord, error) {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return nil, err
	}
	value, ok := driver.(provider.DNSManager)
	if !ok {
		return nil, ErrUnsupported
	}
	return value.UpdateRecord(ctx, &conn, credential, &instance, recordID, record)
}
func (s *RuntimeService) DeleteDNS(ctx context.Context, id, recordID string) error {
	instance, conn, credential, driver, err := s.driver(ctx, id)
	if err != nil {
		return err
	}
	value, ok := driver.(provider.DNSManager)
	if !ok {
		return ErrUnsupported
	}
	return value.DeleteRecord(ctx, &conn, credential, &instance, recordID)
}
