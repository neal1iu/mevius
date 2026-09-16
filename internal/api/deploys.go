package api

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/service"

	"github.com/go-chi/chi/v5"
)

type deployHandler struct {
	deploySvc *service.DeployService
}

func registerDeployRoutes(r chi.Router, deploySvc *service.DeployService) {
	h := &deployHandler{deploySvc: deploySvc}

	r.Route("/bindings/{id}/deploys", func(r chi.Router) {
		r.Post("/", h.trigger)
		r.Get("/", h.list)
		r.Get("/{deployID}/logs", h.logs)
	})
}

type deployResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	InProgress bool   `json:"in_progress"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

func newDeployResponse(e domain.DeployEvent) deployResponse {
	inProgress := e.Status == "queued" || e.Status == "in_progress"
	return deployResponse{
		ID:         e.ID,
		Status:     e.Status,
		InProgress: inProgress,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}
}

func (h *deployHandler) trigger(w http.ResponseWriter, r *http.Request) {
	bindingID := chi.URLParam(r, "id")

	event, err := h.deploySvc.TriggerDeploy(r.Context(), bindingID)
	if err != nil {
		handleDeployError(w, err)
		return
	}

	writeJSON(w, newDeployResponse(*event), http.StatusAccepted)
}

func (h *deployHandler) list(w http.ResponseWriter, r *http.Request) {
	bindingID := chi.URLParam(r, "id")

	events, err := h.deploySvc.ListDeployments(r.Context(), bindingID)
	if err != nil {
		handleDeployError(w, err)
		return
	}

	resp := make([]deployResponse, 0, len(events))
	for _, e := range events {
		resp = append(resp, newDeployResponse(e))
	}
	writeJSON(w, resp, http.StatusOK)
}

func (h *deployHandler) logs(w http.ResponseWriter, r *http.Request) {
	bindingID := chi.URLParam(r, "id")
	deployID := chi.URLParam(r, "deployID")

	tail := 256
	if t := r.URL.Query().Get("tail"); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			tail = n
		}
	}

	chunk, err := h.deploySvc.GetLogs(r.Context(), bindingID, deployID, tail)
	if err != nil {
		handleDeployError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if chunk.Truncated {
		w.Header().Set("X-Truncated", "true")
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, chunk.Lines)
}

func handleDeployError(w http.ResponseWriter, err error) {
	var pErr *provider.Error
	if errors.As(err, &pErr) {
		switch pErr.Kind {
		case provider.KindUnsupported:
			writeJSON(w, providerErrorPayload{
				Kind:    string(pErr.Kind),
				Message: pErr.Error(),
			}, http.StatusNotImplemented)
			return
		case provider.KindNotFound:
			writeJSON(w, providerErrorPayload{
				Kind:    string(pErr.Kind),
				Message: pErr.Error(),
			}, http.StatusNotFound)
			return
		case provider.KindUnauthorized:
			writeJSON(w, providerErrorPayload{
				Kind:    string(pErr.Kind),
				Message: pErr.Error(),
			}, http.StatusUnauthorized)
			return
		case provider.KindRateLimit:
			writeJSON(w, providerErrorPayload{
				Kind:    string(pErr.Kind),
				Message: pErr.Error(),
			}, http.StatusTooManyRequests)
			return
		default:
			writeJSON(w, providerErrorPayload{
				Kind:    string(pErr.Kind),
				Message: pErr.Error(),
			}, http.StatusBadGateway)
			return
		}
	}

	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, "binding not found", http.StatusNotFound)
		return
	}

	slog.Error("deploy operation", slog.Any("error", err))
	writeError(w, "internal error", http.StatusInternalServerError)
}