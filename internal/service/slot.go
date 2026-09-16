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
	errSlotTypeChange    = errors.New("slot type cannot be changed")
)

func (s *SlotService) Create(ctx context.Context, projectID, kind, name string, config json.RawMessage) (domain.Slot, error) {
	if err := validateSlotConfig(kind, config); err != nil {
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
		Type:       kind,
		Name:       name,
		ConfigJson: configStr,
		CreatedAt:  now,
	})
	if err != nil {
		return domain.Slot{}, fmt.Errorf("insert slot: %w", err)
	}

	return domain.Slot{
		ID:        id,
		ProjectID: projectID,
		Name:      name,
		Kind:      domain.ResourceKind(kind),
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
		if err := validateSlotConfig(existing.Type, config); err != nil {
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

	return storeSlotToDomain(existing), nil
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

func validateSlotConfig(kind string, config json.RawMessage) error {
	switch domain.ResourceKind(kind) {
	case domain.ResourceKindRepo:
		var cfg domain.RepoConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("invalid repo config: %w", err)
		}
		return cfg.Validate()
	case domain.ResourceKindCompute:
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
	case domain.ResourceKindDNSDomain:
		return domain.DnsDomainConfig{}.Validate()
	default:
		return fmt.Errorf("unknown slot type: %s", kind)
	}
}