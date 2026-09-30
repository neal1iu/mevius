package catalog

import (
	"encoding/json"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"sort"
	"strings"
)

type registeredType struct {
	Descriptor ResourceTypeDescriptor
	Handler    ResourceHandler
	Views      map[string]ViewHandler
	Actions    map[string]ActionHandler
	Checkers   map[string]OperationChecker
	schemas    map[string]*jsonschema.Schema
}
type Registry struct {
	providers       map[string]Provider
	products        map[string]ProductDescriptor
	types           map[string]*registeredType
	relations       map[string]RelationDescriptor
	relationSchemas map[string]*jsonschema.Schema
	instanceSchemas map[string]*jsonschema.Schema
	scopeSchemas    map[string]map[string]*jsonschema.Schema
}

func NewRegistry() *Registry {
	r := &Registry{providers: map[string]Provider{}, products: map[string]ProductDescriptor{}, types: map[string]*registeredType{}, relations: map[string]RelationDescriptor{}, relationSchemas: map[string]*jsonschema.Schema{}, instanceSchemas: map[string]*jsonschema.Schema{}, scopeSchemas: map[string]map[string]*jsonschema.Schema{}}
	for _, id := range []string{"related_to", "depends_on"} {
		r.RegisterRelation(RelationDescriptor{ID: id, Origin: "user", From: []string{"*"}, To: []string{"*"}, Cardinality: "many", BlocksDeletion: id == "depends_on", Attributes: EmptySchema()})
	}
	return r
}
func compile(s Schema) (*jsonschema.Schema, error) {
	if s.Version < 1 {
		return nil, fmt.Errorf("schema version must be positive")
	}
	var v any
	if e := json.Unmarshal(s.JSON, &v); e != nil {
		return nil, e
	}
	if object, ok := v.(map[string]any); ok {
		if draft, ok := object["$schema"].(string); ok && draft != "https://json-schema.org/draft/2020-12/schema" {
			return nil, fmt.Errorf("schemas must use JSON Schema 2020-12")
		}
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if e := c.AddResource("urn:mevius:schema", v); e != nil {
		return nil, e
	}
	return c.Compile("urn:mevius:schema")
}
func (r *Registry) RegisterRelation(d RelationDescriptor) {
	if d.ID == "" || (d.Origin != "user" && d.Origin != "adapter") || !member([]string{"one", "many"}, d.Cardinality) || len(d.From) == 0 || len(d.To) == 0 {
		panic("invalid relation descriptor")
	}
	if old, ok := r.relations[d.ID]; ok {
		if old.Origin != d.Origin || old.Cardinality != d.Cardinality || old.BlocksDeletion != d.BlocksDeletion || string(old.Attributes.JSON) != string(d.Attributes.JSON) || old.Attributes.Version != d.Attributes.Version {
			panic("conflicting relation descriptor: " + d.ID)
		}
		for _, v := range d.From {
			if !member(old.From, v) {
				old.From = append(old.From, v)
			}
		}
		for _, v := range d.To {
			if !member(old.To, v) {
				old.To = append(old.To, v)
			}
		}
		r.relations[d.ID] = old
		return
	}
	if e := publicSchema(d.Attributes); e != nil {
		panic(e)
	}
	s, e := compile(d.Attributes)
	if e != nil {
		panic(e)
	}
	r.relations[d.ID] = d
	r.relationSchemas[d.ID] = s
}
func (r *Registry) RegisterProvider(p Provider) {
	if p.ID() == "" || r.providers[p.ID()] != nil {
		panic("invalid or duplicate provider")
	}
	if e := publicSchema(p.InstanceSchema()); e != nil {
		panic(e)
	}
	instance, e := compile(p.InstanceSchema())
	if e != nil {
		panic(e)
	}
	r.instanceSchemas[p.ID()] = instance
	r.scopeSchemas[p.ID()] = map[string]*jsonschema.Schema{}
	if len(p.ScopeSchemas()) == 0 {
		panic("provider has no scope schemas")
	}
	for typ, schema := range p.ScopeSchemas() {
		if typ == "" {
			panic("empty scope type")
		}
		if e := publicSchema(schema); e != nil {
			panic(e)
		}
		compiled, e := compile(schema)
		if e != nil {
			panic(e)
		}
		r.scopeSchemas[p.ID()][typ] = compiled
	}
	for _, product := range p.Products() {
		if _, exists := r.products[product.ID]; exists {
			panic("duplicate product ID: " + product.ID)
		}
		r.products[product.ID] = product
		if product.ProviderID != p.ID() || product.ID == "" || len(product.ResourceTypes) == 0 {
			panic("invalid product descriptor")
		}
		for _, d := range product.ResourceTypes {
			if d.ProviderID != p.ID() || d.ProductID != product.ID || d.ID == "" || r.types[d.ID] != nil || len(d.Roles) == 0 || !member([]string{"instance", "scope", "parent"}, d.Identity.Scope) || (d.Identity.Scope == "parent" && d.Identity.ParentType == "") {
				panic("invalid resource type: " + d.ID)
			}
			t := &registeredType{Descriptor: d, Views: map[string]ViewHandler{}, Actions: map[string]ActionHandler{}, Checkers: map[string]OperationChecker{}, schemas: map[string]*jsonschema.Schema{}}
			add := func(k string, s Schema) {
				if _, ok := t.schemas[k]; ok {
					panic("duplicate schema: " + k)
				}
				v, e := compile(s)
				if e != nil {
					panic(fmt.Errorf("%s %s: %w", d.ID, k, e))
				}
				t.schemas[k] = v
			}
			for _, schema := range []Schema{d.Locator, d.Reference, d.Observation} {
				if e := publicSchema(schema); e != nil {
					panic(fmt.Errorf("%s: %w", d.ID, e))
				}
			}
			add("locator", d.Locator)
			add("reference", d.Reference)
			add("observation", d.Observation)
			for _, v := range d.Views {
				if v.ID == "" || v.Capability == "" {
					panic("invalid view")
				}
				add("view:"+v.ID, v.Input)
				add("view-output:"+v.ID, v.Output)
			}
			for _, a := range d.Actions {
				if !member([]string{"", "confirmed", "remote"}, a.Authorization) || (a.Effect == "delete" && a.Authorization == "remote") || a.ID == "" || a.Capability == "" || !member([]string{"scope", "resource"}, a.Target) || !member([]string{"create", "delete", "update"}, a.Effect) || (a.Effect == "create" && a.Target != "scope") || (a.Effect == "delete" && a.Target != "resource") {
					panic("invalid action")
				}
				if e := publicSchema(a.Output); e != nil {
					panic(e)
				}
				add("action:"+a.ID, a.Input)
				add("action-output:"+a.ID, a.Output)
			}
			for _, rel := range d.Relations {
				r.RegisterRelation(rel)
			}
			r.types[d.ID] = t
		}
	}
	r.providers[p.ID()] = p
}
func (r *Registry) RegisterHandler(typeID string, h ResourceHandler, views map[string]ViewHandler, actions map[string]ActionHandler) {
	t := r.types[typeID]
	if t == nil || h == nil || t.Handler != nil {
		panic("invalid resource handler: " + typeID)
	}
	for _, d := range t.Descriptor.Views {
		if views[d.ID] == nil {
			panic("missing view handler: " + d.ID)
		}
	}
	for _, d := range t.Descriptor.Actions {
		if actions[d.ID] == nil {
			panic("missing action handler: " + d.ID)
		}
	}
	if len(views) != len(t.Descriptor.Views) || len(actions) != len(t.Descriptor.Actions) {
		panic("undeclared handler")
	}
	t.Handler = h
	t.Views = views
	t.Actions = actions
}
func (r *Registry) Validate(typeID, kind string, doc Document) error {
	t := r.types[typeID]
	if t == nil {
		return fmt.Errorf("%w: unregistered resource type", ErrForbidden)
	}
	var version int
	switch kind {
	case "locator":
		version = t.Descriptor.Locator.Version
	case "reference":
		version = t.Descriptor.Reference.Version
	case "observation":
		version = t.Descriptor.Observation.Version
	default:
		for _, v := range t.Descriptor.Views {
			if kind == "view:"+v.ID {
				version = v.Input.Version
			}
			if kind == "view-output:"+v.ID {
				version = v.Output.Version
			}
		}
		for _, a := range t.Descriptor.Actions {
			if kind == "action:"+a.ID {
				version = a.Input.Version
			}
			if kind == "action-output:"+a.ID {
				version = a.Output.Version
			}
		}
	}
	if version != doc.Version || t.schemas[kind] == nil {
		return fmt.Errorf("%w: unsupported %s version %d", ErrInvalid, kind, doc.Version)
	}
	var v any
	if e := json.Unmarshal(doc.Data, &v); e != nil {
		return fmt.Errorf("%w: invalid JSON", ErrInvalid)
	}
	if e := t.schemas[kind].Validate(v); e != nil {
		return fmt.Errorf("%w: %s schema validation failed", ErrInvalid, kind)
	}
	return nil
}
func (r *Registry) Products() []ProductDescriptor {
	out := []ProductDescriptor{}
	for _, p := range r.providers {
		out = append(out, p.Products()...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (r *Registry) Relations() []RelationDescriptor {
	out := []RelationDescriptor{}
	for _, d := range r.relations {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func ObjectSchema(properties map[string]any, required ...string) Schema {
	if required == nil {
		required = []string{}
	}
	return Schema{Version: 1, JSON: jsonBytes(map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "properties": properties, "required": required, "additionalProperties": false})}
}
func EmptySchema() Schema { return ObjectSchema(map[string]any{}) }

// Persisted documents must be closed projections, never raw provider payloads.
func publicSchema(schema Schema) error {
	var root any
	if e := json.Unmarshal(schema.JSON, &root); e != nil {
		return e
	}
	var walk func(any) error
	walk = func(node any) error {
		switch v := node.(type) {
		case bool:
			if v {
				return fmt.Errorf("unconstrained public schemas are not permitted")
			}
		case map[string]any:
			if len(v) == 0 {
				return fmt.Errorf("unconstrained public schemas are not permitted")
			}
			if v["type"] == "object" || v["properties"] != nil {
				if v["additionalProperties"] != false {
					return fmt.Errorf("public object schemas must set additionalProperties=false")
				}
			}
			if ref, ok := v["$ref"].(string); ok && !strings.HasPrefix(ref, "#") {
				return fmt.Errorf("external schema references are not permitted")
			}
			if props, ok := v["properties"].(map[string]any); ok {
				for key, child := range props {
					lower := strings.ToLower(key)
					for _, secret := range []string{"password", "secret", "token", "credential", "connection_string", "connection_url"} {
						if strings.Contains(lower, secret) {
							return fmt.Errorf("secret field %s in public schema", key)
						}
					}
					if e := walk(child); e != nil {
						return e
					}
				}
			}
			for key, child := range v {
				if key == "properties" {
					continue
				}
				switch child.(type) {
				case map[string]any, []any:
					if e := walk(child); e != nil {
						return e
					}
				}
			}
		case []any:
			for _, child := range v {
				if e := walk(child); e != nil {
					return e
				}
			}
		}
		return nil
	}
	return walk(root)
}

func (r *Registry) RegisterOperationCheckers(typeID string, checkers map[string]OperationChecker) {
	t := r.types[typeID]
	if t == nil {
		panic("unregistered operation checker type")
	}
	if t.Checkers == nil {
		t.Checkers = map[string]OperationChecker{}
	}
	for id, checker := range checkers {
		if t.Actions[id] == nil || checker == nil || t.Checkers[id] != nil {
			panic("invalid operation checker")
		}
		t.Checkers[id] = checker
	}
}

// ValidateReady is called after registering every compiled integration.
func (r *Registry) ValidateReady() error {
	for id, t := range r.types {
		if t.Handler == nil {
			return fmt.Errorf("%s has no handler", id)
		}
		if t.Descriptor.Identity.Scope == "parent" {
			p := r.types[t.Descriptor.Identity.ParentType]
			if p == nil || p.Descriptor.ProviderID != t.Descriptor.ProviderID {
				return fmt.Errorf("%s has an invalid parent type", id)
			}
		}
	}
	return nil
}

func (r *Registry) ProviderDescriptors() []ProviderDescriptor {
	out := []ProviderDescriptor{}
	for id, p := range r.providers {
		out = append(out, ProviderDescriptor{ID: id, Name: id, Instance: p.InstanceSchema(), Scopes: p.ScopeSchemas()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func validateDocument(compiled *jsonschema.Schema, version int, document Document) error {
	if compiled == nil || version != document.Version {
		return ErrInvalid
	}
	var value any
	if json.Unmarshal(document.Data, &value) != nil {
		return ErrInvalid
	}
	if compiled.Validate(value) != nil {
		return ErrInvalid
	}
	return nil
}
func (r *Registry) ValidateInstance(providerID string, document Document) error {
	p := r.providers[providerID]
	if p == nil {
		return ErrInvalid
	}
	return validateDocument(r.instanceSchemas[providerID], p.InstanceSchema().Version, document)
}
func (r *Registry) ValidateScopeLocator(providerID, typ string, document Document) error {
	p := r.providers[providerID]
	if p == nil {
		return ErrInvalid
	}
	schema, ok := p.ScopeSchemas()[typ]
	if !ok {
		return ErrInvalid
	}
	return validateDocument(r.scopeSchemas[providerID][typ], schema.Version, document)
}
