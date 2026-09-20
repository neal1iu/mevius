package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"mevius/internal/provider"
	"mevius/internal/service"
)

type server struct {
	connections *service.ConnectionService
	projects    *service.ProjectService
	resources   *service.ResourceService
	links       *service.LinkService
	runtime     *service.RuntimeService
	oauth       *service.OAuthService
	registry    *provider.Registry
}

func NewRouter(token string, logger *slog.Logger, connections *service.ConnectionService, projects *service.ProjectService, resources *service.ResourceService, links *service.LinkService, runtime *service.RuntimeService, oauth *service.OAuthService, registry *provider.Registry) http.Handler {
	s := &server{connections: connections, projects: projects, resources: resources, links: links, runtime: runtime, oauth: oauth, registry: registry}
	r := chi.NewRouter()
	r.Use(requestLogger(logger))
	r.Get("/healthz", handleHealth)
	r.Get("/api/v1/connections/oauth/callback/{providerID}", s.oauthCallback)
	r.Route("/api/v1", func(r chi.Router) { r.Use(bearerAuth(token)); s.routes(r) })
	return r
}

func (s *server) routes(r chi.Router) {
	r.Get("/catalog/providers", s.catalog)
	r.Post("/connections/probe", s.probeConnection)
	r.Get("/connections/oauth/providers", s.oauthProviders)
	r.Put("/connections/oauth/providers/{providerID}/configuration", s.saveOAuthConfiguration)
	r.Delete("/connections/oauth/providers/{providerID}/configuration", s.deleteOAuthConfiguration)
	r.Post("/connections/oauth/start", s.oauthStart)
	r.Get("/connections/oauth/sessions/{sessionID}", s.oauthSession)
	r.Post("/connections/oauth/complete", s.oauthComplete)
	r.Get("/connections", s.listConnections)
	r.Post("/connections", s.createConnection)
	r.Get("/connections/{id}", s.getConnection)
	r.Patch("/connections/{id}/credential", s.rotateCredential)
	r.Delete("/connections/{id}", s.deleteConnection)
	r.Get("/connections/{id}/products/{productID}/resources", s.discover)
	r.Get("/projects", s.listProjects)
	r.Post("/projects", s.createProject)
	r.Get("/projects/{id}", s.getProject)
	r.Patch("/projects/{id}", s.updateProject)
	r.Delete("/projects/{id}", s.deleteProject)
	r.Get("/projects/{id}/resources", s.listProjectResources)
	r.Post("/projects/{id}/resources", s.attachProjectResource)
	r.Patch("/projects/{id}/resources/{resourceID}", s.updateProjectResource)
	r.Delete("/projects/{id}/resources/{resourceID}", s.detachProjectResource)
	r.Get("/resource-instances", s.listResources)
	r.Post("/resource-instances", s.createResource)
	r.Post("/resource-instances/import", s.importResource)
	r.Get("/resource-instances/{id}", s.getResource)
	r.Post("/resource-instances/{id}/refresh", s.refreshResource)
	r.Post("/resource-instances/{id}/release", s.releaseResource)
	r.Delete("/resource-instances/{id}", s.forgetResource)
	r.Delete("/resource-instances/{id}/remote", s.deleteRemote)
	r.Get("/resource-relations", s.listRelations)
	r.Post("/resource-relations", s.createRelation)
	r.Delete("/resource-relations/{id}", s.deleteRelation)
	r.Get("/resource-instances/{id}/deployments", s.listDeployments)
	r.Post("/resource-instances/{id}/deployments", s.triggerDeployment)
	r.Get("/resource-instances/{id}/deployments/{executionID}/logs", s.deploymentLogs)
	r.Get("/resource-instances/{id}/pipeline-runs", s.listPipelineRuns)
	r.Post("/resource-instances/{id}/pipeline-runs", s.triggerPipeline)
	r.Get("/resource-instances/{id}/pipeline-runs/{runID}", s.getPipelineRun)
	r.Post("/resource-instances/{id}/pipeline-runs/{runID}/cancel", s.cancelPipeline)
	r.Post("/resource-instances/{id}/pipeline-runs/{runID}/rerun", s.rerunPipeline)
	r.Get("/resource-instances/{id}/pipeline-runs/{runID}/logs", s.pipelineLogs)
	r.Get("/resource-instances/{id}/dns-records", s.listDNS)
	r.Post("/resource-instances/{id}/dns-records", s.createDNS)
	r.Patch("/resource-instances/{id}/dns-records/{recordID}", s.updateDNS)
	r.Delete("/resource-instances/{id}/dns-records/{recordID}", s.deleteDNS)
}

func statusFor(err error) int {
	var providerError *provider.Error
	if errors.As(err, &providerError) {
		switch providerError.Kind {
		case provider.KindUnauthorized:
			return http.StatusUnauthorized
		case provider.KindNotFound:
			return http.StatusNotFound
		case provider.KindRateLimit:
			return http.StatusTooManyRequests
		case provider.KindUnsupported:
			return http.StatusUnprocessableEntity
		}
	}
	switch {
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrConflict), errors.Is(err, service.ErrOperationInProgress), errors.Is(err, service.ErrDuplicateName):
		return http.StatusConflict
	case errors.Is(err, service.ErrUnsupported):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusBadGateway
	}
}
