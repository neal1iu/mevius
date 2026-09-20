package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"mevius/internal/domain"
	"mevius/internal/store"
)

type ProjectService struct{ q *store.Queries }
type ProjectDetail struct {
	Project   domain.Project            `json:"project"`
	Resources []ProjectResourceDetail   `json:"resources"`
	Relations []domain.ResourceRelation `json:"relations"`
}
type ProjectResourceDetail struct {
	ProjectResource  domain.ProjectResource  `json:"project_resource"`
	ResourceInstance domain.ResourceInstance `json:"resource_instance"`
}

var ErrDuplicateName = errors.New("project name already exists")

func NewProjectService(q *store.Queries) *ProjectService { return &ProjectService{q: q} }
func projectFromStore(v store.Project) domain.Project {
	return domain.Project{ID: v.ID, Name: v.Name, Description: v.Description, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func (s *ProjectService) Create(ctx context.Context, name, description string) (domain.Project, error) {
	if name == "" {
		return domain.Project{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if _, err := s.q.GetProjectByName(ctx, name); err == nil {
		return domain.Project{}, ErrDuplicateName
	} else if !errors.Is(err, sql.ErrNoRows) {
		return domain.Project{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	v := domain.Project{ID: uuid.NewString(), Name: name, Description: description, CreatedAt: now, UpdatedAt: now}
	err := s.q.InsertProject(ctx, store.InsertProjectParams{ID: v.ID, Name: name, Description: description, CreatedAt: now, UpdatedAt: now})
	return v, err
}
func (s *ProjectService) List(ctx context.Context) ([]domain.Project, error) {
	rows, err := s.q.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Project, 0, len(rows))
	for _, row := range rows {
		result = append(result, projectFromStore(row))
	}
	return result, nil
}
func (s *ProjectService) Get(ctx context.Context, id string) (domain.Project, error) {
	row, err := s.q.GetProject(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Project{}, ErrNotFound
	}
	return projectFromStore(row), err
}
func (s *ProjectService) Update(ctx context.Context, id, name, description string) (domain.Project, error) {
	current, err := s.Get(ctx, id)
	if err != nil {
		return domain.Project{}, err
	}
	if name == "" {
		name = current.Name
	}
	if existing, e := s.q.GetProjectByName(ctx, name); e == nil && existing.ID != id {
		return domain.Project{}, ErrDuplicateName
	} else if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return domain.Project{}, e
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err = s.q.UpdateProject(ctx, store.UpdateProjectParams{Name: name, Description: description, UpdatedAt: now, ID: id}); err != nil {
		return domain.Project{}, err
	}
	return s.Get(ctx, id)
}
func (s *ProjectService) Delete(ctx context.Context, id string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	return s.q.DeleteProject(ctx, id)
}
func (s *ProjectService) GetDetail(ctx context.Context, id string) (ProjectDetail, error) {
	project, err := s.Get(ctx, id)
	if err != nil {
		return ProjectDetail{}, err
	}
	rows, err := s.q.ListProjectResources(ctx, id)
	if err != nil {
		return ProjectDetail{}, err
	}
	detail := ProjectDetail{Project: project, Resources: make([]ProjectResourceDetail, 0, len(rows))}
	ids := map[string]bool{}
	for _, row := range rows {
		instance, err := s.q.GetResourceInstance(ctx, row.ResourceInstanceID)
		if err != nil {
			return ProjectDetail{}, err
		}
		detail.Resources = append(detail.Resources, ProjectResourceDetail{ProjectResource: projectResourceFromStore(row), ResourceInstance: instanceFromStore(instance)})
		ids[row.ResourceInstanceID] = true
	}
	relations, err := s.q.ListResourceRelations(ctx)
	if err != nil {
		return ProjectDetail{}, err
	}
	for _, r := range relations {
		if ids[r.FromResourceInstanceID] && ids[r.ToResourceInstanceID] {
			detail.Relations = append(detail.Relations, relationFromStore(r))
		}
	}
	return detail, nil
}
