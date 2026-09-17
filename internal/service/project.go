package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"mevius/internal/domain"
	"mevius/internal/store"

	"github.com/google/uuid"
)

type ProjectService struct {
	q store.Querier
}

func NewProjectService(q store.Querier) *ProjectService {
	return &ProjectService{q: q}
}

type ProjectDetail struct {
	Project  domain.Project   `json:"project"`
	Slots    []domain.Slot    `json:"slots"`
	Bindings []domain.Binding `json:"bindings"`
}

var ErrDuplicateName = errors.New("project name already exists")

func (s *ProjectService) Create(ctx context.Context, name, description string) (domain.Project, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.New().String()

	_, err := s.q.GetProjectByName(ctx, name)
	if err == nil {
		return domain.Project{}, ErrDuplicateName
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.Project{}, fmt.Errorf("check project name: %w", err)
	}

	err = s.q.InsertProject(ctx, store.InsertProjectParams{
		ID:          id,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return domain.Project{}, fmt.Errorf("insert project: %w", err)
	}

	return domain.Project{
		ID:          id,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (s *ProjectService) List(ctx context.Context) ([]domain.Project, error) {
	projects, err := s.q.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	result := make([]domain.Project, len(projects))
	for i, p := range projects {
		result[i] = storeProjectToDomain(p)
	}
	return result, nil
}

func (s *ProjectService) Get(ctx context.Context, id string) (domain.Project, error) {
	p, err := s.q.GetProject(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, sql.ErrNoRows
		}
		return domain.Project{}, fmt.Errorf("get project: %w", err)
	}
	return storeProjectToDomain(p), nil
}

func (s *ProjectService) GetDetail(ctx context.Context, id string) (ProjectDetail, error) {
	p, err := s.q.GetProject(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectDetail{}, sql.ErrNoRows
		}
		return ProjectDetail{}, fmt.Errorf("get project detail: %w", err)
	}

	slots, err := s.q.ListSlotsByProject(ctx, id)
	if err != nil {
		return ProjectDetail{}, fmt.Errorf("list slots for detail: %w", err)
	}

	var bindings []domain.Binding
	if len(slots) > 0 {
		slotIDs := make([]string, len(slots))
		for i, sl := range slots {
			slotIDs[i] = sl.ID
		}
		storeBindings, err := s.q.ListBindingsBySlots(ctx, slotIDs)
		if err != nil {
			return ProjectDetail{}, fmt.Errorf("list bindings for detail: %w", err)
		}
		bindings = make([]domain.Binding, len(storeBindings))
		for i, b := range storeBindings {
			bindings[i] = storeBindingToDomain(b)
		}
	}

	domainSlots := make([]domain.Slot, len(slots))
	for i, sl := range slots {
		domainSlots[i] = storeSlotToDomain(sl)
	}

	return ProjectDetail{
		Project:  storeProjectToDomain(p),
		Slots:    domainSlots,
		Bindings: bindings,
	}, nil
}

func (s *ProjectService) Update(ctx context.Context, id, name, description string) (domain.Project, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	if name != "" {
		existing, err := s.q.GetProjectByName(ctx, name)
		if err == nil && existing.ID != id {
			return domain.Project{}, ErrDuplicateName
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, fmt.Errorf("check project name: %w", err)
		}
	}

	err := s.q.UpdateProject(ctx, store.UpdateProjectParams{
		ID:          id,
		Name:        name,
		Description: description,
		UpdatedAt:   now,
	})
	if err != nil {
		return domain.Project{}, fmt.Errorf("update project: %w", err)
	}

	p, err := s.q.GetProject(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, sql.ErrNoRows
		}
		return domain.Project{}, fmt.Errorf("get project after update: %w", err)
	}
	return storeProjectToDomain(p), nil
}

func (s *ProjectService) Delete(ctx context.Context, id string) error {
	err := s.q.DeleteProject(ctx, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

func storeProjectToDomain(p store.Project) domain.Project {
	return domain.Project{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func storeSlotToDomain(s store.Slot) domain.Slot {
	return domain.Slot{
		ID:        s.ID,
		ProjectID: s.ProjectID,
		Name:      s.Name,
		Role:      domain.SlotRole(s.Role),
		Config:    []byte(s.ConfigJson),
		CreatedAt: s.CreatedAt,
	}
}

func storeBindingToDomain(b store.Binding) domain.Binding {
	meta := make(map[string]any)
	if b.CachedMetaJson != "" {
		_ = json.Unmarshal([]byte(b.CachedMetaJson), &meta)
	}
	db := domain.Binding{
		ID:           b.ID,
		SlotID:       b.SlotID,
		ConnectionID: b.ConnectionID,
		ExternalID:   b.ExternalID,
		Product:      domain.ProductType(b.Product),
		CachedMeta:   meta,
		SyncStatus:   domain.SyncStatus(b.SyncStatus),
		LastSyncedAt: "",
		CreatedAt:    b.CreatedAt,
	}
	if b.LastSyncedAt != nil {
		db.LastSyncedAt = *b.LastSyncedAt
	}
	return db
}