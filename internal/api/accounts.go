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

type accountHandlers struct {
	svc *service.AccountService
}

type accountRequest struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	Token    string `json:"token"`
}

type accountResponse struct {
	ID        string         `json:"id"`
	Provider  string         `json:"provider"`
	Label     string         `json:"label"`
	Meta      map[string]any `json:"meta,omitempty"`
	CreatedAt string         `json:"created_at"`
}

type providerErrorPayload struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func newAccountResponse(acct domain.ProviderAccount) accountResponse {
	meta := make(map[string]any)
	if b, err := json.Marshal(acct.Meta); err == nil {
		json.Unmarshal(b, &meta)
	}
	return accountResponse{
		ID:        acct.ID,
		Provider:  string(acct.Provider),
		Label:     acct.Label,
		Meta:      meta,
		CreatedAt: acct.CreatedAt,
	}
}

func (h *accountHandlers) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req accountRequest
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

	acct, err := h.svc.AddAccount(r.Context(), req.Provider, req.Label, req.Token)
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

	writeJSON(w, newAccountResponse(*acct), http.StatusCreated)
}

func (h *accountHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.svc.ListAccounts(r.Context())
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	resp := make([]accountResponse, 0, len(accounts))
	for _, a := range accounts {
		resp = append(resp, newAccountResponse(a))
	}
	writeJSON(w, resp, http.StatusOK)
}

func (h *accountHandlers) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	acct, err := h.svc.GetAccount(r.Context(), id)
	if err != nil {
		writeError(w, "account not found", http.StatusNotFound)
		return
	}
	writeJSON(w, newAccountResponse(*acct), http.StatusOK)
}

func (h *accountHandlers) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.DeleteAccount(r.Context(), id); err != nil {
		writeError(w, "account not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
