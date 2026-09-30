// Package catalog implements the provider-independent resource directory.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalid   = errors.New("invalid catalog request")
	ErrConflict  = errors.New("catalog conflict")
	ErrNotFound  = errors.New("catalog object not found")
	ErrForbidden = errors.New("access path does not permit this operation")
	ErrUnknown   = errors.New("remote operation outcome unknown; do not retry with a new key")
)

type JSON = json.RawMessage

type Document struct {
	Version int  `json:"version"`
	Data    JSON `json:"data"`
}
type Schema struct {
	Version int  `json:"version"`
	JSON    JSON `json:"schema"`
}
type IdentityRule struct {
	Scope      string `json:"scope"`
	Natural    bool   `json:"natural"`
	ParentType string `json:"parent_type,omitempty"`
}
type Capability struct {
	Availability string `json:"availability"`
}
type Capabilities map[string]Capability

type ViewDescriptor struct {
	ID         string `json:"id"`
	Name       string `json:"display_name"`
	Input      Schema `json:"input"`
	Output     Schema `json:"output"`
	Capability string `json:"capability"`
}
type ActionDescriptor struct {
	Authorization string `json:"authorization"`
	ID            string `json:"id"`
	Name          string `json:"display_name"`
	Target        string `json:"target"`
	Input         Schema `json:"input"`
	Output        Schema `json:"output"`
	Capability    string `json:"capability"`
	Effect        string `json:"effect"`
}
type RelationDescriptor struct {
	ID             string   `json:"id"`
	Origin         string   `json:"origin"`
	From           []string `json:"from_types"`
	To             []string `json:"to_types"`
	Cardinality    string   `json:"cardinality"`
	BlocksDeletion bool     `json:"blocks_deletion"`
	Attributes     Schema   `json:"attributes"`
}
type ResourceTypeDescriptor struct {
	ID          string               `json:"id"`
	ProductID   string               `json:"product_id"`
	ProviderID  string               `json:"provider_id"`
	Name        string               `json:"display_name"`
	Category    string               `json:"category"`
	Roles       []string             `json:"compatible_roles"`
	Identity    IdentityRule         `json:"identity"`
	ScopeTypes  []string             `json:"scope_types"`
	Locator     Schema               `json:"locator"`
	Reference   Schema               `json:"reference"`
	Observation Schema               `json:"observation"`
	Views       []ViewDescriptor     `json:"views"`
	Actions     []ActionDescriptor   `json:"actions"`
	Relations   []RelationDescriptor `json:"relations"`
}
type ProductDescriptor struct {
	ID            string                   `json:"id"`
	ProviderID    string                   `json:"provider_id"`
	Name          string                   `json:"display_name"`
	ResourceTypes []ResourceTypeDescriptor `json:"resource_types"`
}
type Instance struct {
	ID         string    `json:"id"`
	ProviderID string    `json:"provider_id"`
	Key        string    `json:"instance_key"`
	Endpoint   Document  `json:"endpoint"`
	CreatedAt  time.Time `json:"created_at"`
}
type Connection struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"provider_instance_id"`
	Label      string    `json:"label"`
	AuthScheme string    `json:"auth_scheme"`
	Revision   int       `json:"credential_revision"`
	Principal  Document  `json:"principal"`
	State      string    `json:"authorization_state"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
type Scope struct {
	ID         string   `json:"id"`
	InstanceID string   `json:"provider_instance_id"`
	Type       string   `json:"scope_type"`
	Key        string   `json:"scope_key"`
	Locator    Document `json:"locator"`
	Label      string   `json:"label"`
	ParentID   *string  `json:"parent_scope_id,omitempty"`
}
type Binding struct {
	ID           string     `json:"id"`
	ConnectionID string     `json:"connection_id"`
	ScopeID      string     `json:"scope_id"`
	Context      Document   `json:"context"`
	State        string     `json:"validation_state"`
	ValidatedAt  *time.Time `json:"validated_at,omitempty"`
}
type Resource struct {
	ID              string    `json:"id"`
	InstanceID      string    `json:"provider_instance_id"`
	TypeID          string    `json:"resource_type_id"`
	IdentityKey     string    `json:"identity_key"`
	IdentityVersion int       `json:"identity_version"`
	ScopeID         string    `json:"scope_id"`
	Locator         Document  `json:"locator"`
	Name            string    `json:"display_name"`
	Origin          string    `json:"origin"`
	Protected       bool      `json:"delete_protection"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
type Access struct {
	ConnectionID              string       `json:"connection_id"`
	ConnectionLabel           string       `json:"connection_label"`
	ScopeLabel                string       `json:"scope_label"`
	ValidationState           string       `json:"validation_state"`
	CurrentCredentialRevision int          `json:"current_credential_revision"`
	ID                        string       `json:"id"`
	ResourceID                string       `json:"resource_id"`
	BindingID                 string       `json:"connection_scope_id"`
	Revision                  int          `json:"credential_revision"`
	Observation               Document     `json:"observation"`
	Capabilities              Capabilities `json:"capabilities"`
	LastAttempt               time.Time    `json:"last_attempt_at"`
	LastSuccess               *time.Time   `json:"last_success_at,omitempty"`
	ErrorCode                 string       `json:"error_code,omitempty"`
}
type Reference struct {
	ID            string   `json:"id,omitempty"`
	ProviderID    string   `json:"provider_id"`
	InstanceID    *string  `json:"provider_instance_id,omitempty"`
	TypeID        string   `json:"resource_type_id"`
	Remote        Document `json:"remote"`
	IdentityParts []string `json:"identity_parts,omitempty"`
	ResolvedID    *string  `json:"resolved_resource_id,omitempty"`
}
type Relation struct {
	ID             string    `json:"id"`
	FromID         string    `json:"from_resource_id"`
	Type           string    `json:"relation_type"`
	Origin         string    `json:"origin"`
	AccessID       *string   `json:"observed_access_id,omitempty"`
	Reference      Reference `json:"reference"`
	Attributes     Document  `json:"attributes"`
	BlocksDeletion bool      `json:"blocks_deletion"`
}
type ObservedRelation struct {
	Type       string
	Reference  Reference
	Attributes JSON
}
type Observation struct {
	IdentityParts     []string           `json:"identity_parts"`
	Locator           JSON               `json:"locator"`
	Name              string             `json:"display_name"`
	Public            JSON               `json:"observation"`
	Capabilities      Capabilities       `json:"capabilities"`
	Relations         []ObservedRelation `json:"-"`
	CompleteRelations []string           `json:"-"`
}
type AccessContext struct {
	Instance   Instance
	Connection Connection
	Scope      Scope
	Binding    Binding
	Credential []byte
}
type ScopeCandidate struct {
	Parent        *ScopeCandidate `json:"parent,omitempty"`
	Type          string          `json:"scope_type"`
	IdentityParts []string        `json:"identity_parts"`
	Locator       JSON            `json:"locator"`
	Label         string          `json:"label"`
	Context       JSON            `json:"context"`
}
type AuthResult struct {
	Principal JSON
	Scopes    []ScopeCandidate
}
type ActionResult struct {
	Public            JSON
	Observation       *Observation
	RemoteOperationID string
	Unknown           bool
	FailureCode       string
}

type ProviderDescriptor struct {
	ID       string            `json:"id"`
	Name     string            `json:"display_name"`
	Instance Schema            `json:"instance"`
	Scopes   map[string]Schema `json:"scopes"`
}

type Provider interface {
	InstanceSchema() Schema
	ScopeSchemas() map[string]Schema
	ID() string
	Validate(context.Context, Instance, []byte) (AuthResult, error)
	ValidateScope(context.Context, AccessContext) (ScopeCandidate, error)
	Products() []ProductDescriptor
}
type ResourceHandler interface {
	Discover(context.Context, AccessContext, JSON) ([]Observation, error)
	Inspect(context.Context, AccessContext, JSON) (Observation, error)
}
type ViewHandler func(context.Context, AccessContext, Resource, Access, JSON) (JSON, error)
type ActionHandler func(context.Context, AccessContext, *Resource, *Access, JSON) (ActionResult, error)

func IdentityKey(parts []string) (string, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("%w: empty identity", ErrInvalid)
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return "", fmt.Errorf("%w: empty identity part", ErrInvalid)
		}
	}
	b, e := json.Marshal(parts)
	return string(b), e
}
func jsonBytes(v any) JSON {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return b
}
func millis(t time.Time) int64    { return t.UTC().UnixMilli() }
func timestamp(v int64) time.Time { return time.UnixMilli(v).UTC() }
func member(a []string, v string) bool {
	for _, s := range a {
		if s == v || s == "*" {
			return true
		}
	}
	return false
}

// ReferenceResolver proves the identity behind a typed remote reference.
type ReferenceResolver interface {
	ResolveReference(context.Context, AccessContext, JSON) (Observation, error)
}

// OperationChecker reads only the result of an already submitted remote write.
type OperationChecker func(context.Context, AccessContext, Operation) (ActionResult, error)
