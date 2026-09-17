package provider

import (
	"context"

	"mevius/internal/domain"
)

type ResourceLister interface {
	ListExternalResources(ctx context.Context, conn *domain.ProviderConnection, credential []byte, kind domain.ResourceKind) ([]domain.ExternalResource, error)
}