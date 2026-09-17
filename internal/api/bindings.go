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

	"github.com/go-chi/chi/v5"
)

type bindingHandler struct {
	bindingSvc *service.BindingService
}

func registerBindingRoutes(r chi.Router, bindingSvc *service.BindingService) {
	h := &bindingHandler{bindingSvc: bindingSvc}

	r.Get("/connections/{id}/discover", h.discover)
	r.Post("/slots/{id}/bindings", h.bind)
	r.Delete("/bindings/{id}", h.unbind)
	r.Post("/bindings/{id}/refresh", h.refresh)
	r.Get("/slots/{id}/bindings", h.listBySlot)
}

func (h *bindingHandler) discover(w http.ResponseWriter, r *http.Request) {
	connectionID := chi.URLParam(r, "id")
	product := r.URL.Query().Get("product")
	if product == "" {
		writeError(w, "product query parameter is required", http.StatusBadRequest)
		return
	}

	resources, err := h.bindingSvc.Discover(r.Context(), connectionID, product)
	if err != nil {
		var pErr *provider.Error
		if errors.As(err, &pErr) {
			writeJSON(w, map[string]string{"error": pErr.Error()}, http.StatusBadRequest)
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "connection not found", http.StatusNotFound)
			return
		}
		slog.Error("discover resources", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if resources == nil {
		resources = []domain.ExternalResource{}
	}
	writeJSON(w, resources, http.StatusOK)
}

type bindRequest struct {
	ConnectionID string `json:"connection_id"`
	Product      string `json:"product"`
	ExternalID   string `json:"external_id"`
}

func (h *bindingHandler) bind(w http.ResponseWriter, r *http.Request) {
	slotID := chi.URLParam(r, "id")

	var req bindRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ConnectionID == "" {
		writeError(w, "connection_id is required", http.StatusBadRequest)
		return
	}
	if req.Product == "" {
		writeError(w, "product is required", http.StatusBadRequest)
		return
	}
	if req.ExternalID == "" {
		writeError(w, "external_id is required", http.StatusBadRequest)
		return
	}

	binding, err := h.bindingSvc.Bind(r.Context(), slotID, req.ConnectionID, req.Product, req.ExternalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "slot or connection not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, service.ErrUnsupportedCombo) {
			writeJSON(w, map[string]string{"error": err.Error()}, http.StatusNotImplemented)
			return
		}
		if errors.Is(err, service.ErrDuplicateBinding) {
			writeError(w, err.Error(), http.StatusConflict)
			return
		}
		slog.Error("bind resource", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, binding, http.StatusCreated)
}

func (h *bindingHandler) unbind(w http.ResponseWriter, r *http.Request) {
	bindingID := chi.URLParam(r, "id")

	if err := h.bindingSvc.Unbind(r.Context(), bindingID); err != nil {
		slog.Error("unbind resource", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *bindingHandler) refresh(w http.ResponseWriter, r *http.Request) {
	bindingID := chi.URLParam(r, "id")

	binding, err := h.bindingSvc.Refresh(r.Context(), bindingID)
	if err != nil {
		slog.Error("refresh binding", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, binding, http.StatusOK)
}

func (h *bindingHandler) listBySlot(w http.ResponseWriter, r *http.Request) {
	slotID := chi.URLParam(r, "id")

	bindings, err := h.bindingSvc.ListBySlot(r.Context(), slotID)
	if err != nil {
		slog.Error("list bindings by slot", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if bindings == nil {
		bindings = []domain.Binding{}
	}
	writeJSON(w, bindings, http.StatusOK)
}
