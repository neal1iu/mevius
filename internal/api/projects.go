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

type projectHandler struct {
	svc *service.ProjectService
}

func registerProjectRoutes(r chi.Router, svc *service.ProjectService) {
	h := &projectHandler{svc: svc}
	r.Route("/projects", func(r chi.Router) {
		r.Post("/", h.create)
		r.Get("/", h.list)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.get)
			r.Patch("/", h.update)
			r.Delete("/", h.delete)
		})
	})
}

type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (h *projectHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		writeError(w, "name is required", http.StatusBadRequest)
		return
	}

	project, err := h.svc.Create(r.Context(), req.Name, req.Description)
	if err != nil {
		if errors.Is(err, service.ErrDuplicateName) {
			writeError(w, "project name already exists", http.StatusConflict)
			return
		}
		slog.Error("create project", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, project, http.StatusCreated)
}

func (h *projectHandler) list(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.List(r.Context())
	if err != nil {
		slog.Error("list projects", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if projects == nil {
		projects = []domain.Project{}
	}
	writeJSON(w, projects, http.StatusOK)
}

func (h *projectHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	detail, err := h.svc.GetDetail(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "project not found", http.StatusNotFound)
			return
		}
		slog.Error("get project detail", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, detail, http.StatusOK)
}

type updateProjectRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (h *projectHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "project not found", http.StatusNotFound)
			return
		}
		slog.Error("get project for update", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}
	desc := existing.Description
	if req.Description != nil {
		desc = *req.Description
	}

	project, err := h.svc.Update(r.Context(), id, name, desc)
	if err != nil {
		if errors.Is(err, service.ErrDuplicateName) {
			writeError(w, "project name already exists", http.StatusConflict)
			return
		}
		slog.Error("update project", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, project, http.StatusOK)
}

func (h *projectHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.svc.Delete(r.Context(), id); err != nil {
		slog.Error("delete project", slog.Any("error", err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode json response", slog.Any("error", err))
	}
}

func writeError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}