package api

import (
	"github.com/go-chi/chi/v5"
	"mevius/internal/catalog"
	"net/http"
)

func (s *catalogServer) oauthRoutes(r chi.Router) {
	r.Get("/connections/oauth/providers", func(w http.ResponseWriter, r *http.Request) { v, e := s.oauth.Providers(r.Context()); respond(w, v, e) })
	r.Put("/connections/oauth/providers/{providerID}/configuration", func(w http.ResponseWriter, r *http.Request) {
		var in catalog.OAuthConfiguration
		if body(w, r, &in) {
			respond(w, nil, s.oauth.Save(r.Context(), chi.URLParam(r, "providerID"), in))
		}
	})
	r.Delete("/connections/oauth/providers/{providerID}/configuration", func(w http.ResponseWriter, r *http.Request) {
		respond(w, nil, s.oauth.Delete(r.Context(), chi.URLParam(r, "providerID")))
	})
	r.Post("/connections/oauth/start", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			InstanceID string `json:"provider_instance_id"`
		}
		if body(w, r, &in) {
			v, e := s.oauth.Start(r.Context(), in.InstanceID)
			respond(w, v, e)
		}
	})
	r.Get("/connections/oauth/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.oauth.Session(r.Context(), id(r))
		respond(w, v, e)
	})
	r.Post("/connections/oauth/complete", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			SessionID string `json:"session_id"`
			Label     string `json:"label"`
		}
		if body(w, r, &in) {
			v, e := s.oauth.Complete(r.Context(), in.SessionID, in.Label)
			respond(w, v, e)
		}
	})
}
func (s *catalogServer) oauthCallback(w http.ResponseWriter, r *http.Request) {
	sessionID, e := s.oauth.Callback(r.Context(), chi.URLParam(r, "providerID"), r.URL.Query().Get("state"), r.URL.Query().Get("code"))
	if e != nil && sessionID == "" {
		respond(w, nil, catalog.ErrInvalid)
		return
	}
	http.Redirect(w, r, s.oauth.ReturnURL(r.Context(), sessionID), http.StatusSeeOther)
}
