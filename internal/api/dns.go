package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/service"
	"mevius/internal/store"

	"github.com/go-chi/chi/v5"
)

type dnsHandler struct {
	q         store.Querier
	reg       *provider.Registry
	engine    *service.RefreshEngine
	credStore domain.CredentialStore
}

func registerDNSRoutes(r chi.Router, q store.Querier, reg *provider.Registry, eng *service.RefreshEngine, credStore domain.CredentialStore) {
	h := &dnsHandler{q: q, reg: reg, engine: eng, credStore: credStore}

	r.Route("/bindings/{id}/dns-records", func(r chi.Router) {
		r.Get("/", h.listRecords)
		r.Post("/", h.createRecord)
		r.Patch("/{recordID}", h.updateRecord)
		r.Delete("/{recordID}", h.deleteRecord)
	})
}

type createDNSRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl,omitempty"`
}

func (h *dnsHandler) resolveBindingAndProvider(r *http.Request) (provider.DNSManager, *domain.ProviderConnection, string, []byte, int, string) {
	bindingID := chi.URLParam(r, "id")

	b, err := h.q.GetBinding(r.Context(), bindingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, "", nil, http.StatusNotFound, "binding not found"
		}
		slog.Error("get binding", slog.Any("error", err))
		return nil, nil, "", nil, http.StatusInternalServerError, "internal error"
	}

	conn, err := h.q.GetProviderConnection(r.Context(), b.ConnectionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, "", nil, http.StatusNotFound, "provider connection not found"
		}
		slog.Error("get provider connection", slog.Any("error", err))
		return nil, nil, "", nil, http.StatusInternalServerError, "internal error"
	}

	p := h.reg.Get(conn.Provider)
	if p == nil {
		slog.Error("provider not registered", slog.String("provider", conn.Provider))
		return nil, nil, "", nil, http.StatusInternalServerError, "internal error"
	}

	dnsMan, ok := p.(provider.DNSManager)
	if !ok {
		slog.Error("provider does not implement DNSManager", slog.String("provider", conn.Provider))
		return nil, nil, "", nil, http.StatusNotImplemented, "provider does not support DNS management"
	}

	cred, err := h.credStore.Resolve(r.Context(), b.ConnectionID)
	if err != nil {
		slog.Error("resolve credential", slog.Any("error", err))
		return nil, nil, "", nil, http.StatusInternalServerError, "internal error"
	}

	provConn := &domain.ProviderConnection{
		ID:       conn.ID,
		Provider: domain.ProviderType(conn.Provider),
		Label:    conn.Label,
		Endpoint: conn.Endpoint,
	}

	return dnsMan, provConn, b.ExternalID, cred, 0, ""
}

func (h *dnsHandler) listRecords(w http.ResponseWriter, r *http.Request) {
	dnsMan, provConn, zoneID, cred, status, msg := h.resolveBindingAndProvider(r)
	if status != 0 {
		writeError(w, msg, status)
		return
	}

	records, err := dnsMan.ListRecords(r.Context(), provConn, cred, zoneID)
	if err != nil {
		var pErr *provider.Error
		if errors.As(err, &pErr) {
			writeJSON(w, map[string]string{"error": pErr.Error()}, http.StatusBadRequest)
			return
		}
		slog.Error("list DNS records", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if records == nil {
		records = []domain.DNSRecord{}
	}
	writeJSON(w, records, http.StatusOK)
}

func (h *dnsHandler) createRecord(w http.ResponseWriter, r *http.Request) {
	bindingID := chi.URLParam(r, "id")

	dnsMan, provConn, zoneID, cred, status, msg := h.resolveBindingAndProvider(r)
	if status != 0 {
		writeError(w, msg, status)
		return
	}

	var req createDNSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Type == "" || req.Name == "" || req.Content == "" {
		writeError(w, "type, name, and content are required", http.StatusBadRequest)
		return
	}

	record := domain.DNSRecord{
		Type:    req.Type,
		Name:    req.Name,
		Content: req.Content,
		TTL:     req.TTL,
	}

	created, err := dnsMan.CreateRecord(r.Context(), provConn, cred, zoneID, record)
	if err != nil {
		var pErr *provider.Error
		if errors.As(err, &pErr) {
			writeJSON(w, map[string]string{"error": pErr.Error()}, http.StatusBadRequest)
			return
		}
		slog.Error("create DNS record", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.engine.Invalidate(bindingID)
	writeJSON(w, created, http.StatusCreated)
}

func (h *dnsHandler) updateRecord(w http.ResponseWriter, r *http.Request) {
	bindingID := chi.URLParam(r, "id")

	dnsMan, provConn, zoneID, cred, status, msg := h.resolveBindingAndProvider(r)
	if status != 0 {
		writeError(w, msg, status)
		return
	}

	recordID := chi.URLParam(r, "recordID")

	var req createDNSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	record := domain.DNSRecord{
		Type:    req.Type,
		Name:    req.Name,
		Content: req.Content,
		TTL:     req.TTL,
	}

	updated, err := dnsMan.UpdateRecord(r.Context(), provConn, cred, zoneID, recordID, record)
	if err != nil {
		var pErr *provider.Error
		if errors.As(err, &pErr) {
			writeJSON(w, map[string]string{"error": pErr.Error()}, http.StatusBadRequest)
			return
		}
		slog.Error("update DNS record", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.engine.Invalidate(bindingID)
	writeJSON(w, updated, http.StatusOK)
}

func (h *dnsHandler) deleteRecord(w http.ResponseWriter, r *http.Request) {
	bindingID := chi.URLParam(r, "id")

	dnsMan, provConn, zoneID, cred, status, msg := h.resolveBindingAndProvider(r)
	if status != 0 {
		writeError(w, msg, status)
		return
	}

	recordID := chi.URLParam(r, "recordID")

	if err := dnsMan.DeleteRecord(r.Context(), provConn, cred, zoneID, recordID); err != nil {
		var pErr *provider.Error
		if errors.As(err, &pErr) {
			writeJSON(w, map[string]string{"error": pErr.Error()}, http.StatusBadRequest)
			return
		}
		slog.Error("delete DNS record", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.engine.Invalidate(bindingID)
	w.WriteHeader(http.StatusNoContent)
}
