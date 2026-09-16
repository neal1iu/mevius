// Package api assembles the mevius HTTP surface: the chi router, the public
// health endpoint, the bearer authentication middleware and request logging.
package api

import (
	"log/slog"
	"net/http"

	"mevius/internal/provider"
	"mevius/internal/service"
	"mevius/internal/store"

	"github.com/go-chi/chi/v5"
)

// NewRouter wires the public health probe, the token-protected /api/v1 group,
// and all business route handlers.
func NewRouter(token string, logger *slog.Logger, accountSvc *service.AccountService, projectSvc *service.ProjectService, slotSvc *service.SlotService, bindingSvc *service.BindingService, deploySvc *service.DeployService, q store.Querier, reg *provider.Registry, eng *service.RefreshEngine) http.Handler {
	r := chi.NewRouter()
	r.Use(requestLogger(logger))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", handleHealth)

		r.Group(func(r chi.Router) {
			r.Use(bearerAuth(token))

			h := &accountHandlers{svc: accountSvc}
			r.Post("/accounts", h.handleCreate)
			r.Get("/accounts", h.handleList)
			r.Get("/accounts/{id}", h.handleGet)
			r.Delete("/accounts/{id}", h.handleDelete)

			registerProjectRoutes(r, projectSvc)
			registerSlotRoutes(r, slotSvc, projectSvc)
			registerBindingRoutes(r, bindingSvc)
			registerDNSRoutes(r, q, reg, eng)
			registerDeployRoutes(r, deploySvc)

			// Catch-all so unmatched /api/v1 paths still pass through auth
			// (chi skips middleware for routes that don't match).
			r.Handle("/*", http.NotFoundHandler())
		})
	})

	return r
}
