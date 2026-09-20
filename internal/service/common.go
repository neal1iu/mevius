package service

import (
	"encoding/json"
	"errors"

	"mevius/internal/domain"
	"mevius/internal/store"
)

var (
	ErrConflict            = errors.New("conflict")
	ErrNotFound            = errors.New("not found")
	ErrInvalid             = errors.New("invalid request")
	ErrUnsupported         = errors.New("unsupported operation")
	ErrOperationInProgress = errors.New("operation in progress")
)

func decode[T any](value string) T {
	var result T
	_ = json.Unmarshal([]byte(value), &result)
	return result
}
func encode(value any) string {
	raw, _ := json.Marshal(value)
	if string(raw) == "null" {
		return "{}"
	}
	return string(raw)
}

func connectionFromStore(v store.ProviderConnection) domain.ProviderConnection {
	return domain.ProviderConnection{ID: v.ID, ProviderID: domain.ProviderID(v.ProviderID), Label: v.Label, Endpoint: v.Endpoint, Scope: domain.ProviderScope{Type: v.ScopeType, ID: v.ScopeID, Label: v.ScopeLabel}, AuthMethod: domain.AuthMethod(v.AuthMethod), Config: decode[map[string]any](v.ConfigJson), RemoteIdentity: decode[map[string]any](v.RemoteIdentityJson), Permissions: decode[map[domain.Capability]domain.CapabilityState](v.PermissionsJson), PermissionsCheckedAt: value(v.PermissionsCheckedAt), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func instanceFromStore(v store.ResourceInstance) domain.ResourceInstance {
	return domain.ResourceInstance{ID: v.ID, ConnectionID: v.ConnectionID, ProviderProductID: domain.ProductID(v.ProviderProductID), ResourceKind: domain.ResourceKind(v.ResourceKind), ExternalID: v.ExternalID, ExternalURL: v.ExternalUrl, DisplayName: v.DisplayName, LifecycleMode: domain.LifecycleMode(v.LifecycleMode), Spec: json.RawMessage(v.SpecJson), ProviderConfig: json.RawMessage(v.ProviderConfigJson), CachedMeta: decode[map[string]any](v.CachedMetaJson), SyncStatus: domain.SyncStatus(v.SyncStatus), Capabilities: decode[map[domain.Capability]domain.CapabilityState](v.CapabilityStateJson), LastSyncedAt: value(v.LastSyncedAt), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func projectResourceFromStore(v store.ProjectResource) domain.ProjectResource {
	return domain.ProjectResource{ID: v.ID, ProjectID: v.ProjectID, ResourceInstanceID: v.ResourceInstanceID, Alias: v.Alias, Purpose: v.Purpose, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func relationFromStore(v store.ResourceRelation) domain.ResourceRelation {
	return domain.ResourceRelation{ID: v.ID, FromResourceInstanceID: v.FromResourceInstanceID, ToResourceInstanceID: v.ToResourceInstanceID, Type: domain.RelationType(v.RelationType), Origin: domain.RelationOrigin(v.Origin), Config: json.RawMessage(v.ConfigJson), CreatedAt: v.CreatedAt}
}
func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
