// Package integrations translates existing provider protocol drivers into the
// versioned catalog contract. Provider-specific identity policies stay here.
package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mevius/internal/catalog"
	"mevius/internal/domain"
	"mevius/internal/provider"
	"strconv"
	"strings"
)

type adapter struct {
	p        provider.Provider
	products []catalog.ProductDescriptor
}
type resourceAdapter struct {
	driver     provider.ProductDriver
	descriptor catalog.ResourceTypeDescriptor
	service    *catalog.Service
}

func raw(v any) catalog.JSON {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return b
}
func str() any     { return map[string]any{"type": "string"} }
func integer() any { return map[string]any{"type": "integer"} }
func boolean() any { return map[string]any{"type": "boolean"} }
func publicSchema() catalog.Schema {
	return catalog.ObjectSchema(map[string]any{"id": str(), "status": str(), "resource_id": str(), "external_url": str()})
}
func locatorSchema() catalog.Schema {
	return catalog.ObjectSchema(map[string]any{"external_id": str(), "remote_id": str(), "external_url": str(), "repository_external_id": str(), "repository_id": str(), "workflow_id": integer(), "workflow_path": str(), "default_ref": str(), "source_repo": str(), "production_branch": str(), "framework": str()}, "external_id")
}
func observationSchema() catalog.Schema {
	return catalog.ObjectSchema(map[string]any{"private": boolean(), "default_branch": str(), "html_url": str(), "state": str(), "path": str(), "modified_on": str(), "created_on": str(), "source_repo": str(), "production_branch": str(), "framework": str(), "source_type": str(), "status": str(), "account_id": str()})
}
func endpoint(i catalog.Instance) string {
	var v struct {
		URL string `json:"url"`
	}
	json.Unmarshal(i.Endpoint.Data, &v)
	return v.URL
}
func caps(v map[domain.Capability]domain.CapabilityState) catalog.Capabilities {
	out := catalog.Capabilities{}
	for k, s := range v {
		out[string(k)] = catalog.Capability{Availability: string(s.Availability)}
	}
	return out
}
func (a *adapter) ID() string                            { return string(a.p.ID()) }
func (a *adapter) Products() []catalog.ProductDescriptor { return a.products }
func (a *adapter) Validate(ctx context.Context, i catalog.Instance, cred []byte) (catalog.AuthResult, error) {
	p, e := a.p.Probe(ctx, endpoint(i), cred)
	if e != nil {
		return catalog.AuthResult{}, e
	}
	identity := map[string]any{}
	for _, k := range []string{"id", "login", "name", "email"} {
		if v, ok := p.Identity[k]; ok {
			identity[k] = v
		}
	}
	if v, ok := p.Identity["user_id"]; ok {
		identity["id"] = v
	}
	if v, ok := p.Identity["username"]; ok {
		identity["login"] = v
	}
	out := catalog.AuthResult{Principal: raw(identity), Scopes: []catalog.ScopeCandidate{}}
	for _, s := range p.Scopes {
		uid := s.ID
		if v, ok := s.Meta["uid"].(string); ok {
			uid = v
		}
		out.Scopes = append(out.Scopes, catalog.ScopeCandidate{Type: s.Type, IdentityParts: []string{s.Type, uid}, Locator: raw(map[string]any{"id": s.ID}), Label: s.Label, Context: raw(caps(p.Permissions))})
	}
	return out, nil
}
func legacyConnection(c catalog.AccessContext) (*domain.ProviderConnection, error) {
	var l struct {
		ID string `json:"id"`
	}
	if e := json.Unmarshal(c.Scope.Locator.Data, &l); e != nil || l.ID == "" {
		return nil, catalog.ErrInvalid
	}
	return &domain.ProviderConnection{ID: c.Connection.ID, ProviderID: domain.ProviderID(c.Instance.ProviderID), Endpoint: endpoint(c.Instance), Scope: domain.ProviderScope{Type: c.Scope.Type, ID: l.ID, Label: c.Scope.Label}}, nil
}
func (a *adapter) ValidateScope(ctx context.Context, c catalog.AccessContext) (catalog.ScopeCandidate, error) {
	conn, e := legacyConnection(c)
	if e != nil {
		return catalog.ScopeCandidate{}, e
	}
	p, e := a.p.ValidateScope(ctx, conn.Endpoint, c.Credential, conn.Scope)
	if e != nil {
		return catalog.ScopeCandidate{}, e
	}
	if len(p.Scopes) != 1 {
		return catalog.ScopeCandidate{}, catalog.ErrInvalid
	}
	sc := p.Scopes[0]
	uid := sc.ID
	if v, ok := sc.Meta["uid"].(string); ok {
		uid = v
	}
	return catalog.ScopeCandidate{Type: sc.Type, IdentityParts: []string{sc.Type, uid}, Locator: raw(map[string]any{"id": sc.ID}), Label: sc.Label, Context: raw(caps(p.Permissions))}, nil
}

func Register(s *catalog.Service, p provider.Provider) {
	a := &adapter{p: p}
	drivers := p.Products()
	for _, driver := range drivers {
		old := driver.Descriptor()
		d := catalog.ResourceTypeDescriptor{ID: string(old.ID), ProductID: string(old.ID), ProviderID: string(p.ID()), Name: old.DisplayName, Category: string(old.ResourceKind), Identity: catalog.IdentityRule{Scope: "instance"}, Locator: locatorSchema(), Reference: locatorSchema(), Observation: observationSchema(), Views: []catalog.ViewDescriptor{}, Actions: []catalog.ActionDescriptor{}, Relations: []catalog.RelationDescriptor{}}
		for _, r := range old.CompatibleRoles {
			d.Roles = append(d.Roles, string(r))
		}
		switch old.ID {
		case "github.repositories":
			d.ScopeTypes = []string{"user", "org"}
		case "github.actions":
			d.ScopeTypes = []string{"user", "org"}
			d.Identity = catalog.IdentityRule{Scope: "parent", ParentType: "github.repositories"}
			d.Relations = append(d.Relations, catalog.RelationDescriptor{ID: "parent", Origin: "adapter", From: []string{"github.actions"}, To: []string{"github.repositories"}, Cardinality: "one", BlocksDeletion: true, Attributes: catalog.EmptySchema()})
		case "cloudflare.workers":
			d.ScopeTypes = []string{"account"}
			d.Identity = catalog.IdentityRule{Scope: "scope", Natural: true}
		case "cloudflare.pages":
			d.ScopeTypes = []string{"account"}
			d.Identity = catalog.IdentityRule{Scope: "scope"}
		case "cloudflare.dns":
			d.ScopeTypes = []string{"account"}
		case "vercel.projects":
			d.ScopeTypes = []string{"personal", "team"}
		case "vercel.dns":
			d.ScopeTypes = []string{"personal", "team"}
			d.Identity = catalog.IdentityRule{Scope: "scope", Natural: true}
		default:
			panic("catalog descriptor required for " + string(old.ID))
		}
		if old.ResourceKind == domain.ResourceKindPage {
			d.Relations = append(d.Relations, catalog.RelationDescriptor{ID: "source_repo", Origin: "adapter", From: []string{"cloudflare.pages", "vercel.projects"}, To: []string{"github.repositories"}, Cardinality: "one", BlocksDeletion: true, Attributes: catalog.EmptySchema()})
		}
		addAction := func(id, name, target, cap, effect string, input catalog.Schema) {
			authorization := "remote"
			if effect == "delete" {
				authorization = "confirmed"
			}
			d.Actions = append(d.Actions, catalog.ActionDescriptor{Authorization: authorization, ID: id, Name: name, Target: target, Capability: cap, Effect: effect, Input: input, Output: publicSchema()})
		}
		if _, ok := driver.(provider.Provisioner); ok {
			props := map[string]any{"name": str()}
			required := []string{"name"}
			if old.ResourceKind == domain.ResourceKindGitRepo {
				props["private"] = boolean()
				props["description"] = str()
			} else {
				for _, k := range []string{"source_repo_instance_id", "production_branch", "framework", "build_command", "output_directory", "root_directory"} {
					props[k] = str()
				}
				required = append(required, "source_repo_instance_id")
			}
			addAction("create", "Create", "scope", "create", "create", catalog.ObjectSchema(props, required...))
			addAction("delete", "Delete remote resource", "resource", "delete", "delete", catalog.EmptySchema())
		}
		listSchema := catalog.Schema{Version: 1, JSON: raw(map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "array", "items": map[string]any{"type": "object"}})}
		addView := func(id, name, cap string, input catalog.Schema, output catalog.Schema) {
			d.Views = append(d.Views, catalog.ViewDescriptor{ID: id, Name: name, Capability: cap, Input: input, Output: output})
		}
		runInput := catalog.ObjectSchema(map[string]any{"run_id": str()}, "run_id")
		logsInput := catalog.ObjectSchema(map[string]any{"execution_id": str(), "tail": map[string]any{"type": "integer", "minimum": 1, "maximum": 1048576}}, "execution_id")
		logsOutput := catalog.ObjectSchema(map[string]any{"lines": str(), "truncated": boolean(), "meta": map[string]any{"type": "object"}}, "lines", "truncated")
		if _, ok := driver.(provider.PipelineRunner); ok {
			addView("runs", "Pipeline runs", "pipeline_runs", catalog.EmptySchema(), listSchema)
			addAction("trigger", "Trigger pipeline", "resource", "pipeline_trigger", "update", catalog.ObjectSchema(map[string]any{"ref": str(), "inputs": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}}))
			addAction("cancel", "Cancel pipeline run", "resource", "pipeline_cancel", "update", runInput)
			addAction("rerun", "Rerun pipeline", "resource", "pipeline_rerun", "update", runInput)
			addView("run", "Pipeline run", "pipeline_runs", runInput, catalog.Schema{Version: 1, JSON: raw(map[string]any{"type": "object"})})
		}
		if _, ok := driver.(provider.Deployer); ok {
			addView("deployments", "Deployments", "deploy", catalog.EmptySchema(), listSchema)
			addAction("deploy", "Deploy", "resource", "deploy", "update", catalog.EmptySchema())
		}
		if _, ok := driver.(provider.PipelineLogSource); ok {
			addView("logs", "Logs", "logs", logsInput, logsOutput)
		} else if _, ok := driver.(provider.DeploymentLogSource); ok {
			addView("logs", "Logs", "logs", logsInput, logsOutput)
		}
		if _, ok := driver.(provider.DNSManager); ok {
			addView("dns_records", "DNS records", "dns_manage", catalog.EmptySchema(), listSchema)
			props := map[string]any{"id": str(), "type": str(), "name": str(), "content": str(), "ttl": integer(), "proxied": boolean()}
			addAction("dns_create", "Create DNS record", "resource", "dns_manage", "update", catalog.ObjectSchema(props, "type", "name", "content"))
			addAction("dns_update", "Update DNS record", "resource", "dns_manage", "update", catalog.ObjectSchema(props, "id", "type", "name", "content"))
			addAction("dns_delete", "Delete DNS record", "resource", "dns_manage", "update", catalog.ObjectSchema(map[string]any{"id": str()}, "id"))
		}
		a.products = append(a.products, catalog.ProductDescriptor{ID: d.ProductID, ProviderID: d.ProviderID, Name: old.DisplayName, ResourceTypes: []catalog.ResourceTypeDescriptor{d}})
	}
	s.Registry.RegisterProvider(a)
	for i, driver := range drivers {
		d := a.products[i].ResourceTypes[0]
		h := &resourceAdapter{driver: driver, descriptor: d, service: s}
		views := map[string]catalog.ViewHandler{}
		actions := map[string]catalog.ActionHandler{}
		for _, v := range d.Views {
			views[v.ID] = h.view(v.ID)
		}
		for _, v := range d.Actions {
			actions[v.ID] = h.action(v.ID)
		}
		s.Registry.RegisterHandler(d.ID, h, views, actions)
		checkers := map[string]catalog.OperationChecker{}
		if _, ok := driver.(provider.Deployer); ok {
			checkers["deploy"] = h.check("deploy")
		}
		if _, ok := driver.(provider.PipelineRunner); ok {
			checkers["trigger"] = h.check("trigger")
			checkers["rerun"] = h.check("rerun")
		}
		s.Registry.RegisterOperationCheckers(d.ID, checkers)

	}
}
func legacyResource(r catalog.Resource, observation catalog.JSON) (*domain.ResourceInstance, error) {
	var locator map[string]any
	if e := json.Unmarshal(r.Locator.Data, &locator); e != nil {
		return nil, e
	}
	external, _ := locator["external_id"].(string)
	v := &domain.ResourceInstance{ID: r.ID, ProviderProductID: domain.ProductID(r.TypeID), ExternalID: external, DisplayName: r.Name, ProviderConfig: r.Locator.Data}
	if remoteID, ok := locator["remote_id"].(string); ok {
		v.IdentityParts = []string{remoteID}
	}
	if e := json.Unmarshal(observation, &v.CachedMeta); e != nil {
		return nil, e
	}
	return v, nil
}
func (h *resourceAdapter) normalize(c catalog.AccessContext, v domain.ExternalResource) (catalog.Observation, error) {
	l := map[string]any{"external_id": v.ExternalID}
	if v.ExternalURL != "" {
		l["external_url"] = v.ExternalURL
	}
	var config map[string]any
	if len(v.ProviderConfig) > 0 {
		if e := json.Unmarshal(v.ProviderConfig, &config); e != nil {
			return catalog.Observation{}, e
		}
	}
	var props map[string]json.RawMessage
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	json.Unmarshal(h.descriptor.Locator.JSON, &schema)
	props = schema.Properties
	for k, x := range config {
		if _, ok := props[k]; ok {
			l[k] = x
		}
	}
	public := map[string]any{}
	json.Unmarshal(h.descriptor.Observation.JSON, &schema)
	for k, x := range v.Meta {
		if _, ok := schema.Properties[k]; ok {
			public[k] = x
		}
		if k == "source_repo" || k == "production_branch" || k == "framework" {
			l[k] = x
		}
	}
	identity := v.IdentityParts
	if strings.HasPrefix(h.descriptor.ID, "github.") {
		for _, part := range identity {
			uid, err := strconv.ParseInt(part, 10, 64)
			if err != nil || uid <= 0 {
				return catalog.Observation{}, catalog.ErrInvalid
			}
		}
	}
	if h.descriptor.ID == "github.repositories" && len(identity) == 1 {
		l["remote_id"] = identity[0]
	}
	if h.descriptor.ID == "cloudflare.pages" && len(identity) == 0 {
		return catalog.Observation{}, fmt.Errorf("Pages project UID missing")
	}
	if len(identity) == 0 {
		identity = []string{v.ExternalID}
	}
	if h.descriptor.Identity.Scope == "scope" {
		var ns []string
		if e := json.Unmarshal([]byte(c.Scope.Key), &ns); e != nil {
			return catalog.Observation{}, e
		}
		identity = append(ns, identity...)
	}
	var auth catalog.Capabilities
	if e := json.Unmarshal(c.Binding.Context.Data, &auth); e != nil {
		return catalog.Observation{}, e
	}
	abilities := catalog.Capabilities{}
	for _, cap := range h.driver.Descriptor().Capabilities {
		state := auth[string(cap)]
		if state.Availability == "" {
			state.Availability = "unknown"
		}
		if remote, ok := v.Capabilities[cap]; ok && remote.Availability == domain.CapabilityUnavailable {
			state.Availability = "unavailable"
		}
		abilities[string(cap)] = state
	}
	abilities["inspect"] = catalog.Capability{Availability: "available"}
	abilities["discover"] = catalog.Capability{Availability: "available"}
	out := catalog.Observation{IdentityParts: identity, Locator: raw(l), Name: v.DisplayName, Public: raw(public), Capabilities: abilities, Relations: []catalog.ObservedRelation{}}
	if h.descriptor.Identity.Scope == "parent" {
		repoID, _ := l["repository_id"].(string)
		repo, _ := l["repository_external_id"].(string)
		if repoID == "" {
			return out, fmt.Errorf("workflow parent identity is missing")
		}
		instanceID := c.Instance.ID
		out.Relations = append(out.Relations, catalog.ObservedRelation{Type: "parent", Reference: catalog.Reference{ProviderID: "github", InstanceID: &instanceID, TypeID: "github.repositories", IdentityParts: []string{repoID}, Remote: catalog.Document{Version: 1, Data: raw(map[string]any{"external_id": repo})}}})
		out.CompleteRelations = append(out.CompleteRelations, "parent")
	}
	if source, ok := public["source_repo"].(string); ok && source != "" {
		out.Relations = append(out.Relations, catalog.ObservedRelation{Type: "source_repo", Reference: catalog.Reference{ProviderID: "github", TypeID: "github.repositories", Remote: catalog.Document{Version: 1, Data: raw(map[string]any{"external_id": source})}}})
		out.CompleteRelations = append(out.CompleteRelations, "source_repo")
	}
	return out, nil
}
func (h *resourceAdapter) Discover(ctx context.Context, c catalog.AccessContext, input catalog.JSON) ([]catalog.Observation, error) {
	conn, e := legacyConnection(c)
	if e != nil {
		return nil, e
	}
	discoverer, ok := h.driver.(provider.Discoverer)
	if !ok {
		return nil, catalog.ErrForbidden
	}
	scope := domain.DiscoveryScope{}
	var request struct {
		ParentLocator catalog.JSON `json:"parent_locator"`
	}
	if e = json.Unmarshal(input, &request); e != nil {
		return nil, e
	}
	if len(request.ParentLocator) > 0 {
		parent, e := legacyResource(catalog.Resource{Locator: catalog.Document{Version: 1, Data: request.ParentLocator}}, catalog.JSON(`{}`))
		if e != nil {
			return nil, e
		}
		parent.ResourceKind = domain.ResourceKindGitRepo
		scope.Parent = parent
	}
	resources, e := discoverer.Discover(ctx, conn, c.Credential, scope)
	if e != nil {
		return nil, e
	}
	out := []catalog.Observation{}
	for _, v := range resources {
		value, e := h.normalize(c, v)
		if e != nil {
			return nil, e
		}
		out = append(out, value)
	}
	return out, nil
}
func (h *resourceAdapter) Inspect(ctx context.Context, c catalog.AccessContext, locator catalog.JSON) (catalog.Observation, error) {
	conn, e := legacyConnection(c)
	if e != nil {
		return catalog.Observation{}, e
	}
	r, e := legacyResource(catalog.Resource{Locator: catalog.Document{Version: 1, Data: locator}}, catalog.JSON(`{}`))
	if e != nil {
		return catalog.Observation{}, e
	}
	inspect, ok := h.driver.(provider.Inspector)
	if !ok {
		return catalog.Observation{}, catalog.ErrForbidden
	}
	v, e := inspect.Inspect(ctx, conn, c.Credential, r)
	if e != nil {
		return catalog.Observation{}, e
	}
	return h.normalize(c, *v)
}
func (h *resourceAdapter) view(id string) catalog.ViewHandler {
	return func(ctx context.Context, c catalog.AccessContext, r catalog.Resource, a catalog.Access, input catalog.JSON) (catalog.JSON, error) {
		conn, e := legacyConnection(c)
		if e != nil {
			return nil, e
		}
		resource, e := legacyResource(r, a.Observation.Data)
		if e != nil {
			return nil, e
		}
		var params struct {
			RunID       string `json:"run_id"`
			ExecutionID string `json:"execution_id"`
			Tail        int    `json:"tail"`
		}
		if e = json.Unmarshal(input, &params); e != nil {
			return nil, e
		}
		var value any
		switch id {
		case "runs":
			value, e = h.driver.(provider.PipelineRunner).ListPipelineRuns(ctx, conn, c.Credential, resource)
		case "run":
			value, e = h.driver.(provider.PipelineRunner).GetPipelineRun(ctx, conn, c.Credential, resource, params.RunID)
		case "deployments":
			value, e = h.driver.(provider.Deployer).ListDeployments(ctx, conn, c.Credential, resource)
		case "dns_records":
			value, e = h.driver.(provider.DNSManager).ListRecords(ctx, conn, c.Credential, resource)
		case "logs":
			if params.Tail == 0 {
				params.Tail = 65536
			}
			if source, ok := h.driver.(provider.PipelineLogSource); ok {
				value, e = source.GetPipelineLogs(ctx, conn, c.Credential, resource, params.ExecutionID, params.Tail)
			} else {
				value, e = h.driver.(provider.DeploymentLogSource).GetDeploymentLogs(ctx, conn, c.Credential, resource, params.ExecutionID, params.Tail)
			}
		}
		if e != nil {
			return nil, e
		}
		return raw(value), nil
	}
}
func (h *resourceAdapter) action(id string) catalog.ActionHandler {
	return func(ctx context.Context, c catalog.AccessContext, r *catalog.Resource, a *catalog.Access, input catalog.JSON) (catalog.ActionResult, error) {
		conn, e := legacyConnection(c)
		if e != nil {
			return catalog.ActionResult{}, e
		}
		var resource *domain.ResourceInstance
		if r != nil {
			resource, e = legacyResource(*r, a.Observation.Data)
			if e != nil {
				return catalog.ActionResult{}, e
			}
		}
		result := catalog.ActionResult{Public: raw(map[string]any{})}
		var execution *domain.Execution
		switch id {
		case "create":
			request := domain.CreateResourceRequest{Spec: input}
			var params struct {
				SourceID string `json:"source_repo_instance_id"`
			}
			if e = json.Unmarshal(input, &params); e != nil {
				return result, e
			}
			if params.SourceID != "" {
				source, e := h.service.Resource(ctx, params.SourceID)
				if e != nil {
					return result, e
				}
				if source.TypeID != "github.repositories" {
					return result, catalog.ErrInvalid
				}
				request.Source, e = legacyResource(*source, catalog.JSON(`{}`))
				if e != nil {
					return result, e
				}
				request.Source.ResourceKind = domain.ResourceKindGitRepo
			}
			v, e := h.driver.(provider.Provisioner).Create(ctx, conn, c.Credential, request)
			if e != nil {
				return result, e
			}
			obs, e := h.normalize(c, *v)
			if e != nil {
				return result, e
			}
			result.Observation = &obs
		case "delete":
			e = h.driver.(provider.Provisioner).Delete(ctx, conn, c.Credential, resource)
		case "deploy":
			var value *domain.Deployment
			value, e = h.driver.(provider.Deployer).TriggerDeployment(ctx, conn, c.Credential, resource)
			if e == nil {
				result.Public = raw(map[string]any{"id": value.ID, "status": value.Status})
				result.RemoteOperationID = value.ID
			}
		case "trigger":
			var p domain.TriggerPipelineRequest
			if e = json.Unmarshal(input, &p); e == nil {
				execution, e = h.driver.(provider.PipelineRunner).TriggerPipeline(ctx, conn, c.Credential, resource, p)
			}
		case "cancel", "rerun":
			var p struct {
				RunID string `json:"run_id"`
			}
			if e = json.Unmarshal(input, &p); e == nil {
				if id == "cancel" {
					e = h.driver.(provider.PipelineRunner).CancelPipelineRun(ctx, conn, c.Credential, resource, p.RunID)
				} else {
					execution, e = h.driver.(provider.PipelineRunner).RerunPipeline(ctx, conn, c.Credential, resource, p.RunID)
				}
			}
		case "dns_create", "dns_update", "dns_delete":
			var record domain.DNSRecord
			if e = json.Unmarshal(input, &record); e == nil {
				manager := h.driver.(provider.DNSManager)
				var value *domain.DNSRecord
				if id == "dns_create" {
					value, e = manager.CreateRecord(ctx, conn, c.Credential, resource, record)
				} else if id == "dns_update" {
					value, e = manager.UpdateRecord(ctx, conn, c.Credential, resource, record.ID, record)
				} else {
					e = manager.DeleteRecord(ctx, conn, c.Credential, resource, record.ID)
				}
				if value != nil {
					result.Public = raw(map[string]any{"id": value.ID})
				}
			}
		default:
			e = catalog.ErrInvalid
		}
		if execution != nil {
			result.Public = raw(map[string]any{"id": execution.ID, "status": execution.Status})
			result.RemoteOperationID = execution.ID
		}
		if e != nil {
			var pe *provider.Error
			if errors.As(e, &pe) {
				switch pe.Kind {
				case provider.KindUnauthorized:
					result.FailureCode = "denied"
				case provider.KindNotFound:
					result.FailureCode = "not_found"
				case provider.KindRateLimit:
					result.FailureCode = "rate_limited"
				case provider.KindUnsupported:
					result.FailureCode = "invalid_input"
				}
			}
		}
		return result, e
	}
}

// Stable remote UID encoding never uses owner/name as a repository identity.
func UID(v int64) []string { return []string{strconv.FormatInt(v, 10)} }
func (h *resourceAdapter) ResolveReference(ctx context.Context, c catalog.AccessContext, reference catalog.JSON) (catalog.Observation, error) {
	return h.Inspect(ctx, c, reference)
}

func (h *resourceAdapter) check(action string) catalog.OperationChecker {
	return func(ctx context.Context, c catalog.AccessContext, operation catalog.Operation) (catalog.ActionResult, error) {
		r, e := h.service.Resource(ctx, operation.TargetID)
		if e != nil {
			return catalog.ActionResult{}, e
		}
		var observation catalog.JSON = raw(map[string]any{})
		all, e := h.service.Accesses(ctx, r.ID)
		if e != nil {
			return catalog.ActionResult{}, e
		}
		for _, a := range all {
			if a.ID == operation.AccessID {
				observation = a.Observation.Data
			}
		}
		resource, e := legacyResource(*r, observation)
		if e != nil {
			return catalog.ActionResult{}, e
		}
		conn, e := legacyConnection(c)
		if e != nil {
			return catalog.ActionResult{}, e
		}
		if action == "deploy" {
			values, e := h.driver.(provider.Deployer).ListDeployments(ctx, conn, c.Credential, resource)
			if e != nil {
				return catalog.ActionResult{}, e
			}
			for _, v := range values {
				if v.ID == operation.RemoteID {
					return catalog.ActionResult{Public: raw(map[string]any{"id": v.ID, "status": v.Status}), RemoteOperationID: v.ID}, nil
				}
			}
			return catalog.ActionResult{}, catalog.ErrNotFound
		}
		v, e := h.driver.(provider.PipelineRunner).GetPipelineRun(ctx, conn, c.Credential, resource, operation.RemoteID)
		if e != nil {
			return catalog.ActionResult{}, e
		}
		return catalog.ActionResult{Public: raw(map[string]any{"id": v.ID, "status": v.Status}), RemoteOperationID: v.ID}, nil
	}
}

func (a *adapter) InstanceSchema() catalog.Schema {
	return catalog.ObjectSchema(map[string]any{"url": map[string]any{"type": "string", "pattern": "^$|^https?://[^@?#[:space:]]+$", "description": "Empty uses the provider's default API endpoint."}})
}
func (a *adapter) ScopeSchemas() map[string]catalog.Schema {
	out := map[string]catalog.Schema{}
	for _, product := range a.products {
		for _, typ := range product.ResourceTypes {
			for _, scopeType := range typ.ScopeTypes {
				out[scopeType] = catalog.ObjectSchema(map[string]any{"id": str()}, "id")
			}
		}
	}
	return out
}
