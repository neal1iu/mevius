package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"
)

type LinkService struct {
	q        *store.Queries
	registry *provider.Registry
}

func NewLinkService(q *store.Queries, registry *provider.Registry) *LinkService {
	return &LinkService{q: q, registry: registry}
}

func (s *LinkService) Attach(ctx context.Context, projectID, instanceID, alias string, role domain.ResourceRole, purpose string) (domain.ProjectResource, error) {
	if alias == "" {
		return domain.ProjectResource{}, fmt.Errorf("%w: alias is required", ErrInvalid)
	}
	if _, err := s.q.GetProject(ctx, projectID); errors.Is(err, sql.ErrNoRows) {
		return domain.ProjectResource{}, ErrNotFound
	} else if err != nil {
		return domain.ProjectResource{}, err
	}
	instance, err := s.q.GetResourceInstance(ctx, instanceID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProjectResource{}, ErrNotFound
	} else if err != nil {
		return domain.ProjectResource{}, err
	}
	if err := s.validateRole(domain.ProductID(instance.ProviderProductID), role); err != nil {
		return domain.ProjectResource{}, err
	}
	if instance.ResourceKind == string(domain.ResourceKindCIPipeline) || instance.ResourceKind == string(domain.ResourceKindPage) {
		source, sourceErr := s.q.GetSourceRelation(ctx, instanceID)
		if sourceErr == nil {
			attached, err := s.q.ListProjectResources(ctx, projectID)
			if err != nil {
				return domain.ProjectResource{}, err
			}
			found := false
			for _, item := range attached {
				if item.ResourceInstanceID == source.ToResourceInstanceID {
					found = true
					break
				}
			}
			if !found {
				return domain.ProjectResource{}, fmt.Errorf("%w: source repository must be attached to project first", ErrConflict)
			}
		} else if !errors.Is(sourceErr, sql.ErrNoRows) {
			return domain.ProjectResource{}, sourceErr
		} else if instance.ResourceKind == string(domain.ResourceKindCIPipeline) {
			return domain.ProjectResource{}, fmt.Errorf("%w: pipeline has no source repository", ErrConflict)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	v := domain.ProjectResource{ID: uuid.NewString(), ProjectID: projectID, ResourceInstanceID: instanceID, Alias: alias, Role: role, Purpose: purpose, CreatedAt: now, UpdatedAt: now}
	err = s.q.InsertProjectResource(ctx, store.InsertProjectResourceParams{ID: v.ID, ProjectID: projectID, ResourceInstanceID: instanceID, Alias: alias, Role: string(role), Purpose: purpose, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return domain.ProjectResource{}, fmt.Errorf("%w: alias or resource already attached: %v", ErrConflict, err)
	}
	return v, nil
}
func (s *LinkService) ListProjectResources(ctx context.Context, projectID string) ([]domain.ProjectResource, error) {
	rows, err := s.q.ListProjectResources(ctx, projectID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ProjectResource, 0, len(rows))
	for _, row := range rows {
		result = append(result, projectResourceFromStore(row))
	}
	return result, nil
}
func (s *LinkService) UpdateProjectResource(ctx context.Context, id, alias string, role domain.ResourceRole, purpose string) (domain.ProjectResource, error) {
	row, err := s.q.GetProjectResource(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProjectResource{}, ErrNotFound
	}
	if err != nil {
		return domain.ProjectResource{}, err
	}
	if alias == "" {
		alias = row.Alias
	}
	if role == "" {
		role = domain.ResourceRole(row.Role)
	}
	instance, err := s.q.GetResourceInstance(ctx, row.ResourceInstanceID)
	if err != nil {
		return domain.ProjectResource{}, err
	}
	if err = s.validateRole(domain.ProductID(instance.ProviderProductID), role); err != nil {
		return domain.ProjectResource{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err = s.q.UpdateProjectResource(ctx, store.UpdateProjectResourceParams{Alias: alias, Role: string(role), Purpose: purpose, UpdatedAt: now, ID: id}); err != nil {
		return domain.ProjectResource{}, fmt.Errorf("%w: alias already exists", ErrConflict)
	}
	updated, err := s.q.GetProjectResource(ctx, id)
	return projectResourceFromStore(updated), err
}
func (s *LinkService) Detach(ctx context.Context, projectID, id string) error {
	link, err := s.q.GetProjectResource(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && link.ProjectID != projectID) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	attached, err := s.q.ListProjectResources(ctx, projectID)
	if err != nil {
		return err
	}
	attachedIDs := make(map[string]struct{}, len(attached))
	for _, item := range attached {
		attachedIDs[item.ResourceInstanceID] = struct{}{}
	}
	dependencies, err := s.q.ListRelationsTo(ctx, link.ResourceInstanceID)
	if err != nil {
		return err
	}
	for _, relation := range dependencies {
		if relation.RelationType == string(domain.RelationSourceRepo) {
			if _, exists := attachedIDs[relation.FromResourceInstanceID]; exists {
				return fmt.Errorf("%w: detach dependent resources before their source repository", ErrConflict)
			}
		}
	}
	affected, err := s.q.DeleteProjectResource(ctx, store.DeleteProjectResourceParams{ID: id, ProjectID: projectID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *LinkService) validateRole(productID domain.ProductID, role domain.ResourceRole) error {
	if !domain.IsResourceRole(role) {
		return fmt.Errorf("%w: invalid resource role %q", ErrUnprocessable, role)
	}
	if s.registry == nil {
		return fmt.Errorf("%w: product registry unavailable", ErrUnprocessable)
	}
	_, driver, err := s.registry.Resolve(productID)
	if err != nil {
		return fmt.Errorf("%w: product %s is not registered", ErrUnprocessable, productID)
	}
	for _, compatible := range driver.Descriptor().CompatibleRoles {
		if role == compatible {
			return nil
		}
	}
	return fmt.Errorf("%w: role %q is incompatible with product %s", ErrUnprocessable, role, productID)
}

func (s *LinkService) CreateRelation(ctx context.Context, from, to string, relationType domain.RelationType, origin domain.RelationOrigin, config any) (domain.ResourceRelation, error) {
	return s.createRelation(ctx, from, to, relationType, origin, config)
}
func (s *LinkService) createRelation(ctx context.Context, from, to string, relationType domain.RelationType, origin domain.RelationOrigin, config any) (domain.ResourceRelation, error) {
	if from == to {
		return domain.ResourceRelation{}, fmt.Errorf("%w: relation cannot reference itself", ErrInvalid)
	}
	source, err := s.q.GetResourceInstance(ctx, from)
	if err != nil {
		return domain.ResourceRelation{}, ErrNotFound
	}
	target, err := s.q.GetResourceInstance(ctx, to)
	if err != nil {
		return domain.ResourceRelation{}, ErrNotFound
	}
	switch relationType {
	case domain.RelationSourceRepo:
		if target.ResourceKind != string(domain.ResourceKindGitRepo) || (source.ResourceKind != string(domain.ResourceKindCIPipeline) && source.ResourceKind != string(domain.ResourceKindPage)) {
			return domain.ResourceRelation{}, fmt.Errorf("%w: invalid source_repo kinds", ErrInvalid)
		}
		if source.ResourceKind == string(domain.ResourceKindCIPipeline) && source.ConnectionID != target.ConnectionID {
			return domain.ResourceRelation{}, fmt.Errorf("%w: pipeline and source repository must use the same connection", ErrInvalid)
		}
	default:
		return domain.ResourceRelation{}, fmt.Errorf("%w: invalid relation type", ErrInvalid)
	}
	if origin == "" {
		origin = domain.RelationUser
	}
	now := time.Now().UTC().Format(time.RFC3339)
	v := domain.ResourceRelation{ID: uuid.NewString(), FromResourceInstanceID: from, ToResourceInstanceID: to, Type: relationType, Origin: origin, Config: []byte(encode(config)), CreatedAt: now}
	err = s.q.InsertResourceRelation(ctx, store.InsertResourceRelationParams{ID: v.ID, FromResourceInstanceID: from, ToResourceInstanceID: to, RelationType: string(relationType), Origin: string(origin), ConfigJson: string(v.Config), CreatedAt: now})
	if err != nil {
		return domain.ResourceRelation{}, fmt.Errorf("%w: duplicate or cardinality violation: %v", ErrConflict, err)
	}
	return v, nil
}
func (s *LinkService) ListRelations(ctx context.Context) ([]domain.ResourceRelation, error) {
	rows, err := s.q.ListResourceRelations(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ResourceRelation, 0, len(rows))
	for _, row := range rows {
		result = append(result, relationFromStore(row))
	}
	return result, nil
}
func (s *LinkService) DeleteRelation(ctx context.Context, id string) error {
	row, err := s.q.GetResourceRelation(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if row.Origin == string(domain.RelationSystem) {
		return fmt.Errorf("%w: system relation cannot be deleted directly", ErrConflict)
	}
	affected, err := s.q.DeleteResourceRelation(ctx, id)
	if affected == 0 && err == nil {
		return ErrNotFound
	}
	return err
}

// ResourceService uses the same relation invariants while provisioning.
func (s *ResourceService) createRelation(ctx context.Context, from, to string, relationType domain.RelationType, origin domain.RelationOrigin, config any) (domain.ResourceRelation, error) {
	return NewLinkService(s.q, s.registry).createRelation(ctx, from, to, relationType, origin, config)
}
