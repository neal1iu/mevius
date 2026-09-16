// Package api assembles the mevius HTTP surface: the chi router, the public
// health endpoint, the bearer authentication middleware and request logging.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NewRouter wires the public health probe and the token-protected /api/v1
// group. Business routes are registered inside the protected group by later
// tasks; this package ships no domain endpoints yet.
func NewRouter(token string, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(requestLogger(logger))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", handleHealth)

		r.Group(func(r chi.Router) {
			r.Use(bearerAuth(token))
			// Catch-all so unmatched /api/v1 paths still pass through auth
			// (chi skips middleware for routes that don't match).
			r.Handle("/*", http.NotFoundHandler())
		})
	})

	return r
}
