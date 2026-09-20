package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"
)

type ResourceFilter struct {
	ConnectionID string
	ProductID    domain.ProductID
	Kind         domain.ResourceKind
	Lifecycle    domain.LifecycleMode
	SyncStatus   domain.SyncStatus
}
type ImportResourceInput struct {
	ConnectionID      string           `json:"connection_id"`
	ProviderProductID domain.ProductID `json:"provider_product_id"`
	ExternalID        string           `json:"external_id"`
	ProviderConfig    json.RawMessage  `json:"provider_config,omitempty"`
	ParentInstanceID  string           `json:"parent_instance_id,omitempty"`
}
type CreateResourceInput struct {
	ConnectionID         string           `json:"connection_id"`
	ProviderProductID    domain.ProductID `json:"provider_product_id"`
	Spec                 json.RawMessage  `json:"spec"`
	ProviderConfig       json.RawMessage  `json:"provider_config,omitempty"`
	SourceRepoInstanceID string           `json:"source_repo_instance_id,omitempty"`
}

type ResourceService struct {
	q           *store.Queries
	registry    *provider.Registry
	credentials domain.CredentialStore
	refresh     singleflight.Group
}

func NewResourceService(q *store.Queries, registry *provider.Registry, credentials domain.CredentialStore) *ResourceService {
	return &ResourceService{q: q, registry: registry, credentials: credentials}
}

func (s *ResourceService) Discover(ctx context.Context, connectionID string, productID domain.ProductID, parentID string) ([]domain.ExternalResource, error) {
	conn, credential, driver, err := s.resolve(ctx, connectionID, productID)
	if err != nil {
		return nil, err
	}
	discoverer, ok := driver.(provider.Discoverer)
	if !ok {
		return nil, ErrUnsupported
	}
	scope := domain.DiscoveryScope{}
	if parentID != "" {
		parent, err := s.Get(ctx, parentID, false)
		if err != nil {
			return nil, err
		}
		scope.Parent = &parent
	}
	return discoverer.Discover(ctx, &conn, credential, scope)
}

func (s *ResourceService) Import(ctx context.Context, input ImportResourceInput) (domain.ResourceInstance, error) {
	conn, credential, driver, err := s.resolve(ctx, input.ConnectionID, input.ProviderProductID)
	if err != nil {
		return domain.ResourceInstance{}, err
	}
	inspector, ok := driver.(provider.Inspector)
	if !ok {
		return domain.ResourceInstance{}, ErrUnsupported
	}
	descriptor := driver.Descriptor()
	if existing, lookupErr := s.q.GetResourceInstanceByRemote(ctx, store.GetResourceInstanceByRemoteParams{ConnectionID: input.ConnectionID, ProviderProductID: string(input.ProviderProductID), ExternalID: input.ExternalID}); lookupErr == nil {
		return instanceFromStore(existing), nil
	} else if !errors.Is(lookupErr, sql.ErrNoRows) {
		return domain.ResourceInstance{}, lookupErr
	}
	temporary := domain.ResourceInstance{ConnectionID: input.ConnectionID, ProviderProductID: input.ProviderProductID, ResourceKind: descriptor.ResourceKind, ExternalID: input.ExternalID, ProviderConfig: input.ProviderConfig}
	external, err := inspector.Inspect(ctx, &conn, credential, &temporary)
	if err != nil {
		return domain.ResourceInstance{}, err
	}
	if len(external.ProviderConfig) == 0 {
		external.ProviderConfig = input.ProviderConfig
	}
	instance, err := s.persist(ctx, input.ConnectionID, descriptor, domain.LifecycleImported, *external)
	if err != nil {
		return domain.ResourceInstance{}, err
	}
	if descriptor.ResourceKind == domain.ResourceKindCIPipeline {
		if input.ParentInstanceID == "" {
			s.q.DeleteResourceInstance(ctx, instance.ID)
			return domain.ResourceInstance{}, fmt.Errorf("%w: pipeline parent_instance_id is required", ErrInvalid)
		}
		if _, err = s.ensureSourceRelation(ctx, instance.ID, input.ParentInstanceID); err != nil {
			s.q.DeleteResourceInstance(ctx, instance.ID)
			return domain.ResourceInstance{}, err
		}
	} else if descriptor.ResourceKind == domain.ResourceKindPage && input.ParentInstanceID != "" {
		if _, err = s.ensureSourceRelation(ctx, instance.ID, input.ParentInstanceID); err != nil {
			s.q.DeleteResourceInstance(ctx, instance.ID)
			return domain.ResourceInstance{}, err
		}
	}
	return instance, nil
}

func (s *ResourceService) Create(ctx context.Context, input CreateResourceInput, idempotencyKey string) (domain.ResourceInstance, error) {
	if idempotencyKey == "" {
		return domain.ResourceInstance{}, fmt.Errorf("%w: Idempotency-Key is required", ErrInvalid)
	}
	requestHash := hash(struct {
		Operation string
		Input     CreateResourceInput
	}{"create", input})
	if previous, err := s.replayCreate(ctx, idempotencyKey, requestHash, input); err != nil || previous != nil {
		if previous != nil {
			return *previous, nil
		}
		return domain.ResourceInstance{}, err
	}
	operationID, err := s.beginOperation(ctx, idempotencyKey, "create_resource", requestHash)
	if err != nil {
		return domain.ResourceInstance{}, err
	}
	conn, credential, driver, err := s.resolve(ctx, input.ConnectionID, input.ProviderProductID)
	if err != nil {
		s.finishOperation(ctx, operationID, "failed", nil, nil, err)
		return domain.ResourceInstance{}, err
	}
	provisioner, ok := driver.(provider.Provisioner)
	if !ok {
		s.finishOperation(ctx, operationID, "failed", nil, nil, ErrUnsupported)
		return domain.ResourceInstance{}, ErrUnsupported
	}
	var source *domain.ResourceInstance
	if driver.Descriptor().ResourceKind == domain.ResourceKindPage {
		sourceID := input.SourceRepoInstanceID
		if sourceID == "" {
			var spec domain.PageSpec
			_ = json.Unmarshal(input.Spec, &spec)
			sourceID = spec.SourceRepoInstanceID
		}
		v, e := s.Get(ctx, sourceID, false)
		if e != nil {
			s.finishOperation(ctx, operationID, "failed", nil, nil, e)
			return domain.ResourceInstance{}, e
		}
		source = &v
	}
	external, err := provisioner.Create(ctx, &conn, credential, domain.CreateResourceRequest{Spec: input.Spec, ProviderConfig: input.ProviderConfig, Source: source})
	if err != nil {
		s.finishOperation(ctx, operationID, "failed", nil, nil, err)
		return domain.ResourceInstance{}, err
	}
	if len(external.Spec) == 0 {
		external.Spec = input.Spec
	}
	if len(external.ProviderConfig) == 0 {
		external.ProviderConfig = input.ProviderConfig
	}
	instance, err := s.persist(ctx, input.ConnectionID, driver.Descriptor(), domain.LifecycleManaged, *external)
	if err == nil && source != nil {
		_, err = s.ensureSourceRelation(ctx, instance.ID, source.ID)
	}
	if err != nil {
		_ = provisioner.Delete(ctx, &conn, credential, &domain.ResourceInstance{ExternalID: external.ExternalID})
		s.finishOperation(ctx, operationID, "failed", nil, nil, err)
		return domain.ResourceInstance{}, err
	}
	response, _ := json.Marshal(instance)
	s.finishOperation(ctx, operationID, "succeeded", &instance.ID, response, nil)
	return instance, nil
}

func (s *ResourceService) replayCreate(ctx context.Context, key, requestHash string, input CreateResourceInput) (*domain.ResourceInstance, error) {
	op, err := s.q.GetOperationRequestByKey(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if op.RequestHash != requestHash {
		return nil, fmt.Errorf("%w: idempotency key was used for a different request", ErrConflict)
	}
	switch op.Status {
	case "succeeded":
		if op.ResourceInstanceID == nil {
			return nil, fmt.Errorf("saved operation has no resource")
		}
		v, err := s.Get(ctx, *op.ResourceInstanceID, false)
		return &v, err
	case "failed":
		return nil, fmt.Errorf("saved operation failed: %s", op.ErrorJson)
	case "unknown":
		return nil, fmt.Errorf("%w: operation outcome is unknown", ErrConflict)
	default:
		created, _ := time.Parse(time.RFC3339, op.CreatedAt)
		if time.Since(created) < 5*time.Second {
			return nil, ErrOperationInProgress
		}
		conn, credential, driver, resolveErr := s.resolve(ctx, input.ConnectionID, input.ProviderProductID)
		if resolveErr != nil {
			s.markUnknown(ctx, op.ID, resolveErr)
			return nil, fmt.Errorf("%w: create recovery could not resolve driver", ErrConflict)
		}
		recovery, ok := driver.(provider.CreateRecovery)
		if !ok {
			s.markUnknown(ctx, op.ID, ErrUnsupported)
			return nil, fmt.Errorf("%w: driver cannot recover create", ErrConflict)
		}
		var source *domain.ResourceInstance
		sourceID := input.SourceRepoInstanceID
		if sourceID == "" && driver.Descriptor().ResourceKind == domain.ResourceKindPage {
			var spec domain.PageSpec
			_ = json.Unmarshal(input.Spec, &spec)
			sourceID = spec.SourceRepoInstanceID
		}
		if sourceID != "" {
			value, sourceErr := s.Get(ctx, sourceID, false)
			if sourceErr != nil {
				s.markUnknown(ctx, op.ID, sourceErr)
				return nil, fmt.Errorf("%w: source repository unavailable during recovery", ErrConflict)
			}
			source = &value
		}
		external, recoveryErr := recovery.RecoverCreate(ctx, &conn, credential, domain.CreateResourceRequest{Spec: input.Spec, ProviderConfig: input.ProviderConfig, Source: source})
		if recoveryErr != nil || external == nil {
			if recoveryErr == nil {
				recoveryErr = errors.New("remote resource could not be uniquely confirmed")
			}
			s.markUnknown(ctx, op.ID, recoveryErr)
			return nil, fmt.Errorf("%w: create outcome is unknown", ErrConflict)
		}
		if len(external.Spec) == 0 {
			external.Spec = input.Spec
		}
		instance, persistErr := s.persist(ctx, input.ConnectionID, driver.Descriptor(), domain.LifecycleManaged, *external)
		if persistErr == nil && source != nil {
			_, persistErr = s.ensureSourceRelation(ctx, instance.ID, source.ID)
		}
		if persistErr != nil {
			s.markUnknown(ctx, op.ID, persistErr)
			return nil, fmt.Errorf("%w: recovered remote resource could not be registered", ErrConflict)
		}
		response, _ := json.Marshal(instance)
		s.finishOperation(ctx, op.ID, "succeeded", &instance.ID, response, nil)
		return &instance, nil
	}
}

func (s *ResourceService) markUnknown(ctx context.Context, id string, operationErr error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_ = s.q.UpdateOperationRequest(ctx, store.UpdateOperationRequestParams{Status: "unknown", ResponseJson: "{}", ErrorJson: encode(map[string]any{"message": operationErr.Error()}), UpdatedAt: now, ExpiresAt: nil, ID: id})
}
func (s *ResourceService) beginOperation(ctx context.Context, key, operation, requestHash string) (string, error) {
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	_ = s.q.DeleteExpiredOperationRequests(ctx, &nowText)
	id := uuid.NewString()
	expires := now.Add(24 * time.Hour).Format(time.RFC3339)
	err := s.q.InsertOperationRequest(ctx, store.InsertOperationRequestParams{ID: id, IdempotencyKey: key, OperationType: operation, RequestHash: requestHash, Status: "in_progress", ResponseJson: "{}", ErrorJson: "{}", CreatedAt: nowText, UpdatedAt: nowText, ExpiresAt: &expires})
	if err != nil {
		return "", fmt.Errorf("%w: duplicate idempotency request", ErrOperationInProgress)
	}
	return id, nil
}
func (s *ResourceService) finishOperation(ctx context.Context, id, status string, resourceID *string, response []byte, operationErr error) {
	now := time.Now().UTC()
	expires := now.Add(24 * time.Hour).Format(time.RFC3339)
	errorJSON := "{}"
	if operationErr != nil {
		errorJSON = encode(map[string]any{"message": operationErr.Error()})
	}
	_ = s.q.UpdateOperationRequest(ctx, store.UpdateOperationRequestParams{Status: status, ResourceInstanceID: resourceID, ResponseJson: string(response), ErrorJson: errorJSON, UpdatedAt: now.Format(time.RFC3339), ExpiresAt: &expires, ID: id})
}

func (s *ResourceService) persist(ctx context.Context, connectionID string, descriptor domain.ProductDescriptor, lifecycle domain.LifecycleMode, external domain.ExternalResource) (domain.ResourceInstance, error) {
	if existing, err := s.q.GetResourceInstanceByRemote(ctx, store.GetResourceInstanceByRemoteParams{ConnectionID: connectionID, ProviderProductID: string(descriptor.ID), ExternalID: external.ExternalID}); err == nil {
		return instanceFromStore(existing), nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return domain.ResourceInstance{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	capabilities := s.effective(ctx, connectionID, descriptor, external.Capabilities)
	v := domain.ResourceInstance{ID: uuid.NewString(), ConnectionID: connectionID, ProviderProductID: descriptor.ID, ResourceKind: descriptor.ResourceKind, ExternalID: external.ExternalID, ExternalURL: external.ExternalURL, DisplayName: external.DisplayName, LifecycleMode: lifecycle, Spec: external.Spec, ProviderConfig: external.ProviderConfig, CachedMeta: external.Meta, SyncStatus: domain.SyncStatusOK, Capabilities: capabilities, LastSyncedAt: now, CreatedAt: now, UpdatedAt: now}
	if len(v.Spec) == 0 {
		v.Spec = json.RawMessage(`{}`)
	}
	if len(v.ProviderConfig) == 0 {
		v.ProviderConfig = json.RawMessage(`{"version":1}`)
	}
	err := s.q.InsertResourceInstance(ctx, store.InsertResourceInstanceParams{ID: v.ID, ConnectionID: connectionID, ProviderProductID: string(descriptor.ID), ResourceKind: string(descriptor.ResourceKind), ExternalID: v.ExternalID, ExternalUrl: v.ExternalURL, DisplayName: v.DisplayName, LifecycleMode: string(lifecycle), SpecJson: string(v.Spec), ProviderConfigJson: string(v.ProviderConfig), CachedMetaJson: encode(v.CachedMeta), SyncStatus: string(v.SyncStatus), CapabilityStateJson: encode(v.Capabilities), LastSyncedAt: &now, CreatedAt: now, UpdatedAt: now})
	return v, err
}

func (s *ResourceService) effective(ctx context.Context, connectionID string, descriptor domain.ProductDescriptor, instance map[domain.Capability]domain.CapabilityState) map[domain.Capability]domain.CapabilityState {
	row, err := s.q.GetProviderConnection(ctx, connectionID)
	permissions := map[domain.Capability]domain.CapabilityState{}
	if err == nil {
		permissions = connectionFromStore(row).Permissions
	}
	result := map[domain.Capability]domain.CapabilityState{}
	for _, capability := range descriptor.Capabilities {
		state, ok := permissions[capability]
		if !ok {
			state = domain.CapabilityState{Availability: domain.CapabilityUnknown, Reason: "permission was not reported by provider"}
		}
		if override, exists := instance[capability]; exists && (override.Availability == domain.CapabilityUnavailable || state.Availability != domain.CapabilityUnavailable) {
			state = override
		}
		result[capability] = state
	}
	return result
}

func (s *ResourceService) resolve(ctx context.Context, connectionID string, productID domain.ProductID) (domain.ProviderConnection, []byte, provider.ProductDriver, error) {
	row, err := s.q.GetProviderConnection(ctx, connectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProviderConnection{}, nil, nil, ErrNotFound
	}
	if err != nil {
		return domain.ProviderConnection{}, nil, nil, err
	}
	conn := connectionFromStore(row)
	p, driver, err := s.registry.Resolve(productID)
	if err != nil {
		return conn, nil, nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if p.ID() != conn.ProviderID {
		return conn, nil, nil, fmt.Errorf("%w: product does not belong to connection provider", ErrInvalid)
	}
	credential, err := s.credentials.Resolve(ctx, connectionID)
	return conn, credential, driver, err
}

func (s *ResourceService) List(ctx context.Context, filter ResourceFilter) ([]domain.ResourceInstance, error) {
	rows, err := s.q.ListResourceInstances(ctx)
	if err != nil {
		return nil, err
	}
	result := []domain.ResourceInstance{}
	for _, row := range rows {
		v := instanceFromStore(row)
		if filter.ConnectionID != "" && v.ConnectionID != filter.ConnectionID || filter.ProductID != "" && v.ProviderProductID != filter.ProductID || filter.Kind != "" && v.ResourceKind != filter.Kind || filter.Lifecycle != "" && v.LifecycleMode != filter.Lifecycle || filter.SyncStatus != "" && v.SyncStatus != filter.SyncStatus {
			continue
		}
		result = append(result, v)
	}
	return result, nil
}
func (s *ResourceService) Get(ctx context.Context, id string, refresh bool) (domain.ResourceInstance, error) {
	row, err := s.q.GetResourceInstance(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ResourceInstance{}, ErrNotFound
	}
	if err != nil {
		return domain.ResourceInstance{}, err
	}
	v := instanceFromStore(row)
	if refresh {
		s.shouldRefresh(ctx, &v)
	}
	return v, nil
}
func (s *ResourceService) shouldRefresh(ctx context.Context, v *domain.ResourceInstance) {
	ttl := 60 * time.Second
	if v.SyncStatus != domain.SyncStatusOK {
		ttl = 30 * time.Second
	}
	last, _ := time.Parse(time.RFC3339, v.LastSyncedAt)
	if time.Since(last) < ttl {
		return
	}
	fresh, err := s.Refresh(ctx, v.ID)
	if err == nil {
		*v = fresh
	}
}
func (s *ResourceService) Refresh(ctx context.Context, id string) (domain.ResourceInstance, error) {
	result, err, _ := s.refresh.Do(id, func() (any, error) {
		row, e := s.q.GetResourceInstance(ctx, id)
		if e != nil {
			return nil, e
		}
		instance := instanceFromStore(row)
		conn, credential, driver, e := s.resolve(ctx, instance.ConnectionID, instance.ProviderProductID)
		if e != nil {
			return nil, e
		}
		inspector, ok := driver.(provider.Inspector)
		if !ok {
			return nil, ErrUnsupported
		}
		external, e := inspector.Inspect(ctx, &conn, credential, &instance)
		now := time.Now().UTC().Format(time.RFC3339)
		if e != nil {
			status := domain.SyncStatusError
			var pe *provider.Error
			if errors.As(e, &pe) && pe.Kind == provider.KindUnauthorized {
				status = domain.SyncStatusAuthError
			}
			_ = s.q.UpdateResourceInstanceSync(ctx, store.UpdateResourceInstanceSyncParams{ExternalUrl: instance.ExternalURL, DisplayName: instance.DisplayName, CachedMetaJson: encode(instance.CachedMeta), SyncStatus: string(status), CapabilityStateJson: encode(instance.Capabilities), LastSyncedAt: &now, UpdatedAt: now, ID: id})
			return nil, e
		}
		caps := s.effective(ctx, instance.ConnectionID, driver.Descriptor(), external.Capabilities)
		e = s.q.UpdateResourceInstanceSync(ctx, store.UpdateResourceInstanceSyncParams{ExternalUrl: external.ExternalURL, DisplayName: external.DisplayName, CachedMetaJson: encode(external.Meta), SyncStatus: string(domain.SyncStatusOK), CapabilityStateJson: encode(caps), LastSyncedAt: &now, UpdatedAt: now, ID: id})
		if e != nil {
			return nil, e
		}
		updated, e := s.q.GetResourceInstance(ctx, id)
		value := instanceFromStore(updated)
		return value, e
	})
	if err != nil {
		return domain.ResourceInstance{}, err
	}
	return result.(domain.ResourceInstance), nil
}
func (s *ResourceService) Release(ctx context.Context, id string) (domain.ResourceInstance, error) {
	v, err := s.Get(ctx, id, false)
	if err != nil {
		return v, err
	}
	if v.LifecycleMode != domain.LifecycleManaged {
		return v, fmt.Errorf("%w: only managed resources can be released", ErrConflict)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	err = s.q.UpdateResourceInstanceLifecycle(ctx, store.UpdateResourceInstanceLifecycleParams{LifecycleMode: string(domain.LifecycleImported), UpdatedAt: now, ID: id})
	if err != nil {
		return v, err
	}
	return s.Get(ctx, id, false)
}
func (s *ResourceService) Forget(ctx context.Context, id string) error {
	v, err := s.Get(ctx, id, false)
	if err != nil {
		return err
	}
	if v.LifecycleMode != domain.LifecycleImported {
		return fmt.Errorf("%w: release managed resource before forgetting it", ErrConflict)
	}
	if err = s.ensureUnreferenced(ctx, id); err != nil {
		return err
	}
	affected, err := s.q.DeleteResourceInstance(ctx, id)
	if affected == 0 && err == nil {
		return ErrNotFound
	}
	return err
}
func (s *ResourceService) DeleteRemote(ctx context.Context, id, idempotencyKey string) error {
	if idempotencyKey == "" {
		return fmt.Errorf("%w: Idempotency-Key is required", ErrInvalid)
	}
	requestHash := hash(struct{ Operation, ID string }{"delete", id})
	if op, lookupErr := s.q.GetOperationRequestByKey(ctx, idempotencyKey); lookupErr == nil {
		if op.RequestHash != requestHash {
			return fmt.Errorf("%w: idempotency key conflict", ErrConflict)
		}
		switch op.Status {
		case "succeeded":
			return nil
		case "failed":
			return fmt.Errorf("saved operation failed: %s", op.ErrorJson)
		case "unknown":
			return fmt.Errorf("%w: delete outcome is unknown", ErrConflict)
		default:
			return ErrOperationInProgress
		}
	} else if !errors.Is(lookupErr, sql.ErrNoRows) {
		return lookupErr
	}
	v, err := s.Get(ctx, id, false)
	if err != nil {
		return err
	}
	if v.LifecycleMode != domain.LifecycleManaged {
		return fmt.Errorf("%w: only managed resources can be remotely deleted", ErrConflict)
	}
	if err = s.ensureUnreferenced(ctx, id); err != nil {
		return err
	}
	opID, err := s.beginOperation(ctx, idempotencyKey, "delete_resource", requestHash)
	if err != nil {
		return err
	}
	conn, credential, driver, err := s.resolve(ctx, v.ConnectionID, v.ProviderProductID)
	if err == nil {
		provisioner, ok := driver.(provider.Provisioner)
		if !ok {
			err = ErrUnsupported
		} else {
			err = provisioner.Delete(ctx, &conn, credential, &v)
			var pe *provider.Error
			if errors.As(err, &pe) && pe.Kind == provider.KindNotFound {
				err = nil
			}
		}
	}
	if err == nil {
		_, err = s.q.DeleteResourceInstance(ctx, id)
	}
	if err != nil {
		s.finishOperation(ctx, opID, "failed", nil, nil, err)
		return err
	}
	s.finishOperation(ctx, opID, "succeeded", nil, []byte(`{}`), nil)
	return nil
}
func (s *ResourceService) ensureUnreferenced(ctx context.Context, id string) error {
	projects, err := s.q.CountProjectResourcesByInstance(ctx, id)
	if err != nil {
		return err
	}
	relations, err := s.q.CountRelationsTo(ctx, id)
	if err != nil {
		return err
	}
	if projects > 0 || relations > 0 {
		return fmt.Errorf("%w: resource is still referenced", ErrConflict)
	}
	return nil
}

func (s *ResourceService) ensureSourceRelation(ctx context.Context, from, to string) (domain.ResourceRelation, error) {
	existing, err := s.q.GetSourceRelation(ctx, from)
	if err == nil {
		if existing.ToResourceInstanceID != to {
			return domain.ResourceRelation{}, fmt.Errorf("%w: resource already has a different source repository", ErrConflict)
		}
		return relationFromStore(existing), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.ResourceRelation{}, err
	}
	return s.createRelation(ctx, from, to, domain.RelationSourceRepo, domain.RelationSystem, nil)
}

func hash(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
