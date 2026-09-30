// Package catalogtest supplies a compiled multi-resource provider contract.
// It deliberately has no dependency on core resource kinds or provider switches.
package catalogtest

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"mevius/internal/catalog"
	"mevius/internal/store"
	"path/filepath"
	"sync"
	"testing"
)

func Raw(v any) catalog.JSON {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return b
}
func Doc(v map[string]any) catalog.Document { return catalog.Document{Version: 1, Data: Raw(v)} }
func Text() any                             { return map[string]any{"type": "string"} }

type Provider struct {
	ProviderID    string
	Descriptors   []catalog.ProductDescriptor
	Configuration *catalog.Schema
}

func (p *Provider) ID() string                            { return p.ProviderID }
func (p *Provider) Products() []catalog.ProductDescriptor { return p.Descriptors }
func Caps(write bool) catalog.Capabilities {
	state := "unavailable"
	if write {
		state = "available"
	}
	return catalog.Capabilities{"inspect": {Availability: "available"}, "create": {Availability: state}, "update": {Availability: state}, "delete": {Availability: state}}
}
func (p *Provider) Validate(_ context.Context, _ catalog.Instance, cred []byte) (catalog.AuthResult, error) {
	if string(cred) == "bad" {
		return catalog.AuthResult{}, errors.New("password=upstream-secret")
	}
	return catalog.AuthResult{Principal: Raw(map[string]any{"id": "principal", "token": "must-not-persist"}), Scopes: []catalog.ScopeCandidate{{Type: "account", IdentityParts: []string{"account", "A", "live"}, Locator: Raw(map[string]any{"account": "A", "environment": "live"}), Label: "A live"}}}, nil
}
func (p *Provider) ValidateScope(_ context.Context, c catalog.AccessContext) (catalog.ScopeCandidate, error) {
	var locator struct {
		Account     string `json:"account"`
		Environment string `json:"environment"`
	}
	if e := json.Unmarshal(c.Scope.Locator.Data, &locator); e != nil || locator.Account == "" || locator.Environment == "" {
		return catalog.ScopeCandidate{}, catalog.ErrInvalid
	}
	return catalog.ScopeCandidate{Type: "account", IdentityParts: []string{"account", locator.Account, locator.Environment}, Locator: Raw(map[string]any{"account": locator.Account, "environment": locator.Environment}), Label: locator.Account + " " + locator.Environment, Context: Raw(Caps(string(c.Credential) != "read"))}, nil
}

type Fixture struct {
	Service      *catalog.Service
	Provider     *Provider
	mu           sync.Mutex
	Observations map[string]catalog.Observation
	Failures     map[string]bool
	Calls        map[string]int
	Block        chan struct{}
	Entered      chan struct{}
	Unknown      bool
}
type Handler struct {
	F    *Fixture
	Type catalog.ResourceTypeDescriptor
}

func (h *Handler) Inspect(_ context.Context, c catalog.AccessContext, locator catalog.JSON) (catalog.Observation, error) {
	h.F.mu.Lock()
	defer h.F.mu.Unlock()
	if h.F.Failures[c.Connection.ID] {
		return catalog.Observation{}, errors.New("connection-string=postgres://user:secret@host")
	}
	var v struct {
		ID string `json:"id"`
	}
	if e := json.Unmarshal(locator, &v); e != nil {
		return catalog.Observation{}, e
	}
	o, ok := h.F.Observations[h.Type.ID+":"+v.ID]
	if !ok {
		return catalog.Observation{}, catalog.ErrNotFound
	}
	o.Capabilities = Caps(string(c.Credential) != "read")
	if h.Type.Identity.Scope == "scope" {
		var ns []string
		if e := json.Unmarshal([]byte(c.Scope.Key), &ns); e != nil {
			return catalog.Observation{}, e
		}
		o.IdentityParts = append(ns, o.IdentityParts...)
	}
	return o, nil
}
func (h *Handler) Discover(ctx context.Context, c catalog.AccessContext, input catalog.JSON) ([]catalog.Observation, error) {
	v, e := h.Inspect(ctx, c, input)
	if e != nil {
		return nil, e
	}
	return []catalog.Observation{v}, nil
}
func (h *Handler) action(id string) catalog.ActionHandler {
	return func(_ context.Context, c catalog.AccessContext, r *catalog.Resource, _ *catalog.Access, input catalog.JSON) (catalog.ActionResult, error) {
		h.F.mu.Lock()
		h.F.Calls[id]++
		unknown := h.F.Unknown
		block, entered := h.F.Block, h.F.Entered
		h.F.mu.Unlock()
		if entered != nil {
			select {
			case entered <- struct{}{}:
			default:
			}
		}
		if block != nil {
			<-block
		}
		if unknown {
			return catalog.ActionResult{}, errors.New("webhook signing_secret=never-log")
		}
		result := catalog.ActionResult{Public: Raw(map[string]any{})}
		if id == "create" {
			var v struct {
				ID       string `json:"id"`
				Password string `json:"password"`
			}
			if e := json.Unmarshal(input, &v); e != nil {
				return result, e
			}
			o := catalog.Observation{IdentityParts: []string{v.ID}, Locator: Raw(map[string]any{"id": v.ID}), Name: v.ID, Public: Raw(map[string]any{"label": "created"}), Capabilities: Caps(true)}
			if h.Type.Identity.Scope == "scope" {
				var ns []string
				json.Unmarshal([]byte(c.Scope.Key), &ns)
				o.IdentityParts = append(ns, o.IdentityParts...)
			}
			h.F.mu.Lock()
			h.F.Observations[h.Type.ID+":"+v.ID] = catalog.Observation{IdentityParts: []string{v.ID}, Locator: o.Locator, Name: o.Name, Public: o.Public}
			h.F.mu.Unlock()
			result.Observation = &o
		}
		return result, nil
	}
}
func New(t testing.TB) *Fixture {
	t.Helper()
	db, e := store.Open(filepath.Join(t.TempDir(), "catalog.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	r := catalog.NewRegistry()
	key := [32]byte{1, 2, 3}
	f := &Fixture{Service: catalog.NewService(db, r, key), Provider: &Provider{ProviderID: "fixture"}, Observations: map[string]catalog.Observation{}, Failures: map[string]bool{}, Calls: map[string]int{}}
	locator := catalog.ObjectSchema(map[string]any{"id": Text()}, "id")
	observation := catalog.ObjectSchema(map[string]any{"label": Text()})
	parent := catalog.RelationDescriptor{ID: "parent", Origin: "adapter", From: []string{"fixture.branch", "fixture.database"}, To: []string{"fixture.project", "fixture.branch"}, Cardinality: "one", BlocksDeletion: true, Attributes: catalog.EmptySchema()}
	source := catalog.RelationDescriptor{ID: "source_repo", Origin: "adapter", From: []string{"fixture.webhook"}, To: []string{"fixture.project"}, Cardinality: "one", BlocksDeletion: true, Attributes: catalog.EmptySchema()}
	desc := []catalog.ResourceTypeDescriptor{}
	for _, name := range []string{"project", "branch", "database", "webhook"} {
		d := catalog.ResourceTypeDescriptor{ID: "fixture." + name, ProductID: "fixture.neon", ProviderID: "fixture", Name: name, Category: name, Roles: []string{"future_role"}, Identity: catalog.IdentityRule{Scope: "instance"}, ScopeTypes: []string{"account"}, Locator: locator, Reference: locator, Observation: observation, Views: []catalog.ViewDescriptor{}, Actions: []catalog.ActionDescriptor{}, Relations: []catalog.RelationDescriptor{}}
		if name == "branch" || name == "database" {
			d.Identity.Scope = "parent"
			d.Identity.ParentType = "fixture.project"
			if name == "database" {
				d.Identity.ParentType = "fixture.branch"
			}
			d.Relations = []catalog.RelationDescriptor{parent}
		}
		if name == "webhook" {
			d.Identity = catalog.IdentityRule{Scope: "scope", Natural: true}
			d.Relations = []catalog.RelationDescriptor{source}
		}
		for _, a := range []string{"create", "delete", "update"} {
			input := catalog.EmptySchema()
			target := "resource"
			if a == "create" {
				target = "scope"
				input = catalog.ObjectSchema(map[string]any{"id": Text(), "password": Text()}, "id")
			}
			d.Actions = append(d.Actions, catalog.ActionDescriptor{ID: a, Name: a, Target: target, Input: input, Output: catalog.ObjectSchema(map[string]any{"resource_id": Text()}), Capability: a, Effect: a})
		}
		desc = append(desc, d)
	}
	f.Provider.Descriptors = []catalog.ProductDescriptor{{ID: "fixture.neon", ProviderID: "fixture", Name: "Neon fixture", ResourceTypes: desc}}
	r.RegisterProvider(f.Provider)
	for _, d := range desc {
		h := &Handler{f, d}
		actions := map[string]catalog.ActionHandler{}
		for _, a := range d.Actions {
			actions[a.ID] = h.action(a.ID)
		}
		r.RegisterHandler(d.ID, h, map[string]catalog.ViewHandler{}, actions)
	}
	return f
}
func (f *Fixture) Connection(t testing.TB, instanceID, token string) *catalog.Connection {
	t.Helper()
	c, e := f.Service.CreateConnection(context.Background(), instanceID, token, "token", []byte(token))
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func (f *Fixture) Instance(t testing.TB, key string) *catalog.Instance {
	t.Helper()
	v, e := f.Service.CreateInstance(context.Background(), "fixture", key, Doc(map[string]any{"url": ""}))
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func (f *Fixture) Bind(t testing.TB, c *catalog.Connection, account, environment string) *catalog.Binding {
	t.Helper()
	b, e := f.Service.BindScope(context.Background(), c.ID, catalog.ScopeCandidate{Type: "account", IdentityParts: []string{"untrusted", "input"}, Locator: Raw(map[string]any{"account": account, "environment": environment, "token": "stripped"}), Label: "untrusted"})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func (f *Fixture) Put(typ, id string, parts []string, relations ...catalog.ObservedRelation) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Observations[typ+":"+id] = catalog.Observation{IdentityParts: parts, Locator: Raw(map[string]any{"id": id}), Name: id, Public: Raw(map[string]any{"label": id}), Relations: relations}
}
func (f *Fixture) Import(t testing.TB, b *catalog.Binding, typ, id string) *catalog.Resource {
	t.Helper()
	v, e := f.Service.Import(context.Background(), b.ID, typ, Doc(map[string]any{"id": id}))
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func (f *Fixture) Op(t testing.TB, r *catalog.Resource, accessID, action, key string) *catalog.Operation {
	t.Helper()
	v, e := f.Service.Submit(context.Background(), key, catalog.OperationInput{TypeID: r.TypeID, ActionID: action, TargetKind: "resource", TargetID: r.ID, AccessID: accessID, Input: Doc(map[string]any{})})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func Reference(instanceID, typeID, id string, identity []string) catalog.Reference {
	return catalog.Reference{ProviderID: "fixture", InstanceID: &instanceID, TypeID: typeID, IdentityParts: identity, Remote: Doc(map[string]any{"id": id})}
}
func ID() string { return uuid.NewString() }
func (h *Handler) ResolveReference(ctx context.Context, c catalog.AccessContext, reference catalog.JSON) (catalog.Observation, error) {
	return h.Inspect(ctx, c, reference)
}

func (p *Provider) InstanceSchema() catalog.Schema {
	if p.Configuration != nil {
		return *p.Configuration
	}
	return catalog.ObjectSchema(map[string]any{"url": Text()})
}
func (p *Provider) ScopeSchemas() map[string]catalog.Schema {
	return map[string]catalog.Schema{"account": catalog.ObjectSchema(map[string]any{"account": Text(), "environment": Text()}, "account", "environment")}
}
