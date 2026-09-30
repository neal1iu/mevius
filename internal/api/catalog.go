package api

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"io"
	"log/slog"
	"mevius/internal/catalog"
	"net/http"
)

type catalogServer struct {
	service *catalog.Service
	oauth   *catalog.OAuthService
}

func NewCatalogRouter(token string, logger *slog.Logger, s *catalog.Service, oauth *catalog.OAuthService) http.Handler {
	server := &catalogServer{s, oauth}
	r := chi.NewRouter()
	r.Use(requestLogger(logger))
	r.Get("/healthz", handleHealth)
	r.Get("/api/v1/connections/oauth/callback/{providerID}", server.oauthCallback)
	r.Route("/api/v1", func(r chi.Router) { r.Use(bearerAuth(token)); server.routes(r) })
	return r
}
func respond(w http.ResponseWriter, value any, e error) {
	w.Header().Set("Content-Type", "application/json")
	if e != nil {
		status := http.StatusBadGateway
		message := "provider request or catalog persistence failed"
		switch {
		case errors.Is(e, catalog.ErrInvalid):
			status = 400
			message = e.Error()
		case errors.Is(e, catalog.ErrNotFound):
			status = 404
			message = catalog.ErrNotFound.Error()
		case errors.Is(e, catalog.ErrForbidden):
			status = 403
			message = catalog.ErrForbidden.Error()
		case errors.Is(e, catalog.ErrConflict):
			status = 409
			message = e.Error()
		case errors.Is(e, catalog.ErrUnknown):
			status = 409
			message = catalog.ErrUnknown.Error()
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	if value == nil {
		w.WriteHeader(204)
		return
	}
	json.NewEncoder(w).Encode(value)
}
func body(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if e := dec.Decode(v); e != nil {
		respond(w, nil, catalog.ErrInvalid)
		return false
	}
	var trailing any
	if e := dec.Decode(&trailing); e != io.EOF {
		respond(w, nil, catalog.ErrInvalid)
		return false
	}
	return true
}
func id(r *http.Request) string { return chi.URLParam(r, "id") }
func (s *catalogServer) routes(r chi.Router) {
	r.Get("/catalog", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]any{"providers": s.service.Registry.ProviderDescriptors(), "products": s.service.Registry.Products(), "relations": s.service.Registry.Relations()}, nil)
	})
	r.Get("/provider-instances", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.service.Instances(r.Context())
		respond(w, v, e)
	})
	r.Post("/provider-instances", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProviderID string           `json:"provider_id"`
			Key        string           `json:"instance_key"`
			Endpoint   catalog.Document `json:"endpoint"`
		}
		if body(w, r, &in) {
			v, e := s.service.CreateInstance(r.Context(), in.ProviderID, in.Key, in.Endpoint)
			respond(w, v, e)
		}
	})
	r.Get("/connections", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.service.Connections(r.Context())
		respond(w, v, e)
	})
	r.Post("/connections", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			InstanceID string `json:"provider_instance_id"`
			Label      string `json:"label"`
			Credential string `json:"credential"`
		}
		if body(w, r, &in) {
			v, e := s.service.CreateConnection(r.Context(), in.InstanceID, in.Label, "token", []byte(in.Credential))
			respond(w, v, e)
		}
	})
	r.Get("/connections/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.service.Connection(r.Context(), id(r))
		respond(w, v, e)
	})
	r.Patch("/connections/{id}/credential", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Credential string `json:"credential"`
		}
		if body(w, r, &in) {
			v, e := s.service.RotateCredential(r.Context(), id(r), []byte(in.Credential))
			respond(w, v, e)
		}
	})
	r.Delete("/connections/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, nil, s.service.DeleteConnection(r.Context(), id(r)))
	})
	r.Get("/connections/{id}/scopes", func(w http.ResponseWriter, r *http.Request) {
		available, e := s.service.DiscoverScopes(r.Context(), id(r))
		if e != nil {
			respond(w, nil, e)
			return
		}
		bindings, e := s.service.Bindings(r.Context(), id(r))
		if e != nil {
			respond(w, nil, e)
			return
		}
		scopes := []catalog.Scope{}
		for _, b := range bindings {
			sc, e := s.service.Scope(r.Context(), b.ScopeID)
			if e != nil {
				respond(w, nil, e)
				return
			}
			scopes = append(scopes, *sc)
		}
		respond(w, map[string]any{"available": available, "bindings": bindings, "scopes": scopes}, nil)
	})
	r.Post("/connections/{id}/scopes", func(w http.ResponseWriter, r *http.Request) {
		var in catalog.ScopeCandidate
		if body(w, r, &in) {
			v, e := s.service.BindScope(r.Context(), id(r), in)
			respond(w, v, e)
		}
	})
	r.Post("/connection-scopes/{id}/resource-types/{typeID}/discover", func(w http.ResponseWriter, r *http.Request) {
		var in catalog.Document
		if body(w, r, &in) {
			v, e := s.service.Discover(r.Context(), id(r), chi.URLParam(r, "typeID"), in)
			respond(w, v, e)
		}
	})
	r.Get("/resource-instances", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.service.Resources(r.Context())
		respond(w, v, e)
	})
	r.Post("/resource-instances/import", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			BindingID string           `json:"connection_scope_id"`
			TypeID    string           `json:"resource_type_id"`
			Locator   catalog.Document `json:"locator"`
			ConfirmID string           `json:"confirm_resource_id,omitempty"`
		}
		if body(w, r, &in) {
			v, e := s.service.ImportWithConfirmation(r.Context(), in.BindingID, in.TypeID, in.Locator, in.ConfirmID)
			respond(w, v, e)
		}
	})
	r.Get("/resource-instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.service.Resource(r.Context(), id(r))
		if e != nil {
			respond(w, nil, e)
			return
		}
		accesses, e := s.service.Accesses(r.Context(), id(r))
		if e != nil {
			respond(w, nil, e)
			return
		}
		relations, e := s.service.Relations(r.Context(), id(r))
		if e != nil {
			respond(w, nil, e)
			return
		}
		var selected *catalog.Access
		if aid := r.URL.Query().Get("access_id"); aid != "" {
			selected, _, e = s.service.SelectAccess(r.Context(), v, aid)
			if e != nil {
				respond(w, nil, e)
				return
			}
		}
		respond(w, map[string]any{"resource": v, "accesses": accesses, "selected_access": selected, "relations": relations}, nil)
	})
	r.Get("/resource-instances/{id}/accesses", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.service.Accesses(r.Context(), id(r))
		respond(w, v, e)
	})
	r.Post("/resource-instances/{id}/refresh", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			AccessID string `json:"access_id"`
		}
		if body(w, r, &in) {
			v, e := s.service.Refresh(r.Context(), id(r), in.AccessID)
			respond(w, v, e)
		}
	})
	r.Patch("/resource-instances/{id}/protection", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Protected bool `json:"delete_protection"`
		}
		if body(w, r, &in) {
			v, e := s.service.SetProtection(r.Context(), id(r), in.Protected)
			respond(w, v, e)
		}
	})
	r.Delete("/resource-instances/{id}", func(w http.ResponseWriter, r *http.Request) { respond(w, nil, s.service.Forget(r.Context(), id(r))) })
	r.Get("/resource-instances/{id}/views/{viewID}", func(w http.ResponseWriter, r *http.Request) {
		in := catalog.Document{Version: 1, Data: catalog.JSON(`{}`)}
		if value := r.URL.Query().Get("input"); value != "" {
			if json.Unmarshal([]byte(value), &in) != nil {
				respond(w, nil, catalog.ErrInvalid)
				return
			}
		}
		v, e := s.service.View(r.Context(), id(r), r.URL.Query().Get("access_id"), chi.URLParam(r, "viewID"), in)
		respond(w, v, e)
	})
	r.Post("/operations", func(w http.ResponseWriter, r *http.Request) {
		var in catalog.OperationInput
		if body(w, r, &in) {
			v, e := s.service.Submit(r.Context(), r.Header.Get("Idempotency-Key"), in)
			respond(w, v, e)
		}
	})
	r.Get("/operations", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.service.Operations(r.Context())
		respond(w, v, e)
	})
	r.Post("/operations/{id}/check", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.service.CheckOperation(r.Context(), id(r))
		respond(w, v, e)
	})
	r.Get("/operations/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.service.Operation(r.Context(), id(r))
		respond(w, v, e)
	})
	r.Post("/resource-references/{id}/resolve", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			BindingID string `json:"connection_scope_id"`
		}
		if body(w, r, &in) {
			v, e := s.service.ResolveReference(r.Context(), id(r), in.BindingID)
			respond(w, v, e)
		}
	})
	r.Post("/resource-relations", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			FromID     string            `json:"from_resource_id"`
			Type       string            `json:"relation_type"`
			Reference  catalog.Reference `json:"reference"`
			Attributes catalog.Document  `json:"attributes"`
		}
		if body(w, r, &in) {
			respond(w, nil, s.service.AddRelation(r.Context(), in.FromID, in.Type, in.Reference, in.Attributes))
		}
	})
	r.Delete("/resource-relations/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, nil, s.service.RemoveRelation(r.Context(), id(r)))
	})
	r.Get("/projects", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.service.Projects(r.Context())
		respond(w, v, e)
	})
	r.Post("/projects", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if body(w, r, &in) {
			v, e := s.service.SaveProject(r.Context(), "", in.Name, in.Description)
			respond(w, v, e)
		}
	})
	r.Get("/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.service.Project(r.Context(), id(r))
		if e != nil {
			respond(w, nil, e)
			return
		}
		links, e := s.service.ProjectResources(r.Context(), id(r))
		respond(w, map[string]any{"project": v, "resources": links}, e)
	})
	r.Patch("/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if body(w, r, &in) {
			v, e := s.service.SaveProject(r.Context(), id(r), in.Name, in.Description)
			respond(w, v, e)
		}
	})
	r.Delete("/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, nil, s.service.DeleteProject(r.Context(), id(r)))
	})
	r.Post("/projects/{id}/resources", func(w http.ResponseWriter, r *http.Request) {
		var in catalog.ProjectResource
		if body(w, r, &in) {
			in.ProjectID = id(r)
			v, e := s.service.Attach(r.Context(), in)
			respond(w, v, e)
		}
	})
	r.Patch("/projects/{id}/resources/{linkID}", func(w http.ResponseWriter, r *http.Request) {
		var in catalog.ProjectResource
		if body(w, r, &in) {
			respond(w, nil, s.service.UpdateAttachment(r.Context(), id(r), chi.URLParam(r, "linkID"), in))
		}
	})
	r.Delete("/projects/{id}/resources/{linkID}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, nil, s.service.Detach(r.Context(), id(r), chi.URLParam(r, "linkID")))
	})
	s.oauthRoutes(r)
}
