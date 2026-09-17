package api

import (
	"log/slog"
	"net/http"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/service"
	"mevius/internal/store"

	"github.com/go-chi/chi/v5"
)

func NewRouter(token string, logger *slog.Logger, connectionSvc *service.ConnectionService, projectSvc *service.ProjectService, slotSvc *service.SlotService, bindingSvc *service.BindingService, deploySvc *service.DeployService, q store.Querier, reg *provider.Registry, refreshEng *service.RefreshEngine, credStore domain.CredentialStore) http.Handler {
	r := chi.NewRouter()
	r.Use(requestLogger(logger))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", handleHealth)

		r.Group(func(r chi.Router) {
			r.Use(bearerAuth(token))

			registerConnectionRoutes(r, connectionSvc)
			registerProjectRoutes(r, projectSvc)
			registerSlotRoutes(r, slotSvc, projectSvc)
			registerBindingRoutes(r, bindingSvc)
			registerDNSRoutes(r, q, reg, refreshEng, credStore)
			registerDeployRoutes(r, deploySvc)

			r.Handle("/*", http.NotFoundHandler())
		})
	})

	return r
}
