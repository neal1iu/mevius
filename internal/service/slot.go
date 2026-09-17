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

type SlotService struct {
	q *store.Queries
}

func NewSlotService(q *store.Queries) *SlotService {
	return &SlotService{q: q}
}

var (
	ErrSlotConfigInvalid = errors.New("slot config validation failed")
)

func (s *SlotService) Create(ctx context.Context, projectID, role string, config json.RawMessage) (domain.Slot, error) {
	if err := validateSlotConfig(domain.SlotRole(role), config); err != nil {
		return domain.Slot{}, fmt.Errorf("%w: %w", ErrSlotConfigInvalid, err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.New().String()
	configStr := string(config)
	if configStr == "" || configStr == "null" {
		configStr = "{}"
	}

	err := s.q.InsertSlot(ctx, store.InsertSlotParams{
		ID:         id,
		ProjectID:  projectID,
		Role:       role,
		Name:       "",
		ConfigJson: configStr,
		CreatedAt:  now,
	})
	if err != nil {
		return domain.Slot{}, fmt.Errorf("insert slot: %w", err)
	}

	return domain.Slot{
		ID:        id,
		ProjectID: projectID,
		Name:      "",
		Role:      domain.SlotRole(role),
		Config:    config,
		CreatedAt: now,
	}, nil
}

func (s *SlotService) ListByProject(ctx context.Context, projectID string) ([]domain.Slot, error) {
	slots, err := s.q.ListSlotsByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list slots: %w", err)
	}
	result := make([]domain.Slot, len(slots))
	for i, sl := range slots {
		result[i] = storeSlotToDomain(sl)
	}
	return result, nil
}

func (s *SlotService) Get(ctx context.Context, id string) (domain.Slot, error) {
	sl, err := s.q.GetSlot(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Slot{}, sql.ErrNoRows
		}
		return domain.Slot{}, fmt.Errorf("get slot: %w", err)
	}
	return storeSlotToDomain(sl), nil
}

func (s *SlotService) Update(ctx context.Context, id, name string, config json.RawMessage) (domain.Slot, error) {
	existing, err := s.q.GetSlot(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Slot{}, sql.ErrNoRows
		}
		return domain.Slot{}, fmt.Errorf("get slot for update: %w", err)
	}

	if config != nil {
		if err := validateSlotConfig(domain.SlotRole(existing.Role), config); err != nil {
			return domain.Slot{}, fmt.Errorf("%w: %w", ErrSlotConfigInvalid, err)
		}
	}

	updateName := name
	if updateName == "" {
		updateName = existing.Name
	}
	configStr := string(config)
	if config == nil || configStr == "" || configStr == "null" {
		configStr = existing.ConfigJson
	}

	err = s.q.UpdateSlot(ctx, store.UpdateSlotParams{
		ID:         id,
		Name:       updateName,
		ConfigJson: configStr,
	})
	if err != nil {
		return domain.Slot{}, fmt.Errorf("update slot: %w", err)
	}

	updated := storeSlotToDomain(existing)
	updated.Name = updateName
	if config != nil {
		updated.Config = config
	}
	return updated, nil
}

func (s *SlotService) Delete(ctx context.Context, id string) error {
	_, err := s.q.GetSlot(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("get slot for delete: %w", err)
	}

	err = s.q.DeleteSlot(ctx, id)
	if err != nil {
		return fmt.Errorf("delete slot: %w", err)
	}
	return nil
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

func storeSlotToPtr(s store.Slot) *domain.Slot {
	ds := &domain.Slot{
		ID:        s.ID,
		ProjectID: s.ProjectID,
		Name:      s.Name,
		Role:      domain.SlotRole(s.Role),
		CreatedAt: s.CreatedAt,
	}
	if s.ConfigJson != "" && s.ConfigJson != "{}" {
		ds.Config = json.RawMessage(s.ConfigJson)
	}
	return ds
}

func validateSlotConfig(role domain.SlotRole, config json.RawMessage) error {
	kind, ok := roleResourceKind(role)
	if !ok {
		return fmt.Errorf("unknown slot role: %s", role)
	}
	switch kind {
	case domain.ResourceKindRepository:
		var cfg domain.RepoConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("invalid repo config: %w", err)
		}
		return cfg.Validate()
	case domain.ResourceKindWorker:
		var cfg domain.ComputeConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("invalid compute config: %w", err)
		}
		return cfg.Validate()
	case domain.ResourceKindStaticSite:
		var cfg domain.StaticSiteConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("invalid static site config: %w", err)
		}
		return cfg.Validate()
	case domain.ResourceKindDNSZone:
		return domain.DnsDomainConfig{}.Validate()
	default:
		return fmt.Errorf("unknown resource kind: %s", kind)
	}
}

func roleResourceKind(role domain.SlotRole) (domain.ResourceKind, bool) {
	switch role {
	case domain.SlotRoleSource:
		return domain.ResourceKindRepository, true
	case domain.SlotRoleFrontend:
		return domain.ResourceKindStaticSite, true
	case domain.SlotRoleBackend:
		return domain.ResourceKindWorker, true
	case domain.SlotRoleDatabase:
		return domain.ResourceKindWorker, true
	case domain.SlotRoleDNS:
		return domain.ResourceKindDNSZone, true
	default:
		return "", false
	}
}
