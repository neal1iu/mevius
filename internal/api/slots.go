package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"mevius/internal/domain"
	"mevius/internal/service"

	"github.com/go-chi/chi/v5"
)

type slotHandler struct {
	slotSvc    *service.SlotService
	projectSvc *service.ProjectService
}

func registerSlotRoutes(r chi.Router, slotSvc *service.SlotService, projectSvc *service.ProjectService) {
	h := &slotHandler{slotSvc: slotSvc, projectSvc: projectSvc}

	r.Route("/projects/{pid}/slots", func(r chi.Router) {
		r.Post("/", h.create)
		r.Get("/", h.listByProject)
	})

	r.Route("/slots/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.Patch("/", h.update)
		r.Delete("/", h.delete)
	})
}

type createSlotRequest struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config,omitempty"`
}

func (h *slotHandler) create(w http.ResponseWriter, r *http.Request) {
	pid := chi.URLParam(r, "pid")

	var req createSlotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		writeError(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		writeError(w, "type is required", http.StatusBadRequest)
		return
	}
	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	slot, err := h.slotSvc.Create(r.Context(), pid, req.Type, req.Name, req.Config)
	if err != nil {
		if errors.Is(err, service.ErrSlotConfigInvalid) {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "project not found", http.StatusNotFound)
			return
		}
		slog.Error("create slot", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, slot, http.StatusCreated)
}

func (h *slotHandler) listByProject(w http.ResponseWriter, r *http.Request) {
	pid := chi.URLParam(r, "pid")

	_, err := h.projectSvc.Get(r.Context(), pid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "project not found", http.StatusNotFound)
			return
		}
		slog.Error("get project for slot list", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	slots, err := h.slotSvc.ListByProject(r.Context(), pid)
	if err != nil {
		slog.Error("list slots", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if slots == nil {
		slots = []domain.Slot{}
	}
	writeJSON(w, slots, http.StatusOK)
}

func (h *slotHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	slot, err := h.slotSvc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "slot not found", http.StatusNotFound)
			return
		}
		slog.Error("get slot", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, slot, http.StatusOK)
}

type updateSlotRequest struct {
	Name   *string         `json:"name,omitempty"`
	Config *json.RawMessage `json:"config,omitempty"`
}

func (h *slotHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req updateSlotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	slot, err := h.slotSvc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "slot not found", http.StatusNotFound)
			return
		}
		slog.Error("get slot for update", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	name := slot.Name
	if req.Name != nil {
		name = *req.Name
	}

	var config json.RawMessage
	if req.Config != nil {
		config = *req.Config
	}

	updated, err := h.slotSvc.Update(r.Context(), id, name, config)
	if err != nil {
		if errors.Is(err, service.ErrSlotConfigInvalid) {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "slot not found", http.StatusNotFound)
			return
		}
		slog.Error("update slot", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, updated, http.StatusOK)
}

func (h *slotHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.slotSvc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "slot not found", http.StatusNotFound)
			return
		}
		slog.Error("delete slot", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}