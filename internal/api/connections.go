package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/service"

	"github.com/go-chi/chi/v5"
)

type connectionHandlers struct {
	svc *service.ConnectionService
}

type connectionRequest struct {
	Provider string         `json:"provider"`
	Label    string         `json:"label"`
	Endpoint string         `json:"endpoint,omitempty"`
	Token    string         `json:"token"`
	Config   map[string]any `json:"config,omitempty"`
}

type connectionResponse struct {
	ID        string         `json:"id"`
	Provider  string         `json:"provider"`
	Label     string         `json:"label"`
	Endpoint  string         `json:"endpoint,omitempty"`
	Config    map[string]any `json:"config,omitempty"`
	CreatedAt string         `json:"created_at"`
}

type providerErrorPayload struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func newConnectionResponse(conn domain.ProviderConnection) connectionResponse {
	return connectionResponse{
		ID:        conn.ID,
		Provider:  string(conn.Provider),
		Label:     conn.Label,
		Endpoint:  conn.Endpoint,
		Config:    conn.Config,
		CreatedAt: conn.CreatedAt,
	}
}

func (h *connectionHandlers) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req connectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Provider == "" {
		writeError(w, "provider is required", http.StatusBadRequest)
		return
	}
	if req.Token == "" {
		writeError(w, "token is required", http.StatusBadRequest)
		return
	}

	conn, err := h.svc.AddConnection(r.Context(), req.Provider, req.Label, req.Endpoint, []byte(req.Token), req.Config)
	if err != nil {
		var pErr *provider.Error
		if errors.As(err, &pErr) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(providerErrorPayload{Kind: string(pErr.Kind), Message: pErr.Error()})
			return
		}
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, newConnectionResponse(*conn), http.StatusCreated)
}

func (h *connectionHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	connections, err := h.svc.ListConnections(r.Context())
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	resp := make([]connectionResponse, 0, len(connections))
	for _, c := range connections {
		resp = append(resp, newConnectionResponse(c))
	}
	writeJSON(w, resp, http.StatusOK)
}

func (h *connectionHandlers) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	conn, err := h.svc.GetConnection(r.Context(), id)
	if err != nil {
		writeError(w, "connection not found", http.StatusNotFound)
		return
	}
	writeJSON(w, newConnectionResponse(*conn), http.StatusOK)
}

func (h *connectionHandlers) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.DeleteConnection(r.Context(), id); err != nil {
		writeError(w, "connection not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func registerConnectionRoutes(r chi.Router, connectionSvc *service.ConnectionService) {
	h := &connectionHandlers{svc: connectionSvc}
	r.Post("/connections", h.handleCreate)
	r.Get("/connections", h.handleList)
	r.Get("/connections/{id}", h.handleGet)
	r.Delete("/connections/{id}", h.handleDelete)
}
