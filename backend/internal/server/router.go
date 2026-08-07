// Package server assembles the HTTP router and owns the server lifecycle.
// It is the composition root of the delivery layer: every dependency is
// injected here, so no other package reaches for global state.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/BurakKaanKahraman/abacus/backend/internal/auth"
	"github.com/BurakKaanKahraman/abacus/backend/internal/config"
	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/handler"
	"github.com/BurakKaanKahraman/abacus/backend/internal/httpx"
	"github.com/BurakKaanKahraman/abacus/backend/internal/middleware"
	"github.com/BurakKaanKahraman/abacus/backend/internal/usecase"
	"github.com/go-chi/chi/v5"
)

// Router is the application's HTTP handler together with the resources it owns.
type Router struct {
	http.Handler
	rateLimiter *middleware.RateLimiter
}

// Close releases the resources held by the router.
func (r *Router) Close() error {
	return r.rateLimiter.Close()
}

// NewRouter wires middleware, routes and handlers.
//
// Middleware order is deliberate: identity first so every later log line and
// problem document can be correlated, recovery next so a panic anywhere below
// is still rendered as JSON, then the security headers that must be present on
// every response including errors, and only then the per-request work.
func NewRouter(cfg *config.Config, calculator *usecase.Calculator, tokens *auth.TokenService, startedAt time.Time, logger *slog.Logger) *Router {
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitPerMinute, cfg.RateLimitBurst, cfg.TrustProxyHeaders)

	calculateHandler := handler.NewCalculateHandler(calculator)
	healthHandler := handler.NewHealthHandler(startedAt, cfg.Version)
	authHandler := handler.NewAuthHandler(tokens, cfg.ClientID, cfg.ClientSecret)

	r := chi.NewRouter()
	r.Use(
		middleware.RequestID,
		middleware.Recoverer(logger),
		middleware.Logger(logger),
		middleware.SecurityHeaders(cfg.IsProduction()),
		middleware.CORS(cfg.AllowedOrigins),
		middleware.BodyLimit(cfg.MaxRequestBodyBytes),
	)

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteProblem(w, req, domain.NewNotFoundError("The requested resource does not exist."))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteProblem(w, req, domain.NewMethodNotAllowedError(
			"The "+req.Method+" method is not supported by this resource."))
	})

	r.Route("/api/v1", func(api chi.Router) {
		// Health is exempt from rate limiting so that orchestrator probes can
		// never be throttled into reporting a healthy service as down.
		api.Get("/health", healthHandler.Handle)

		api.Group(func(limited chi.Router) {
			limited.Use(rateLimiter.Middleware)

			limited.Post("/auth/token", authHandler.Handle)

			limited.Group(func(protected chi.Router) {
				protected.Use(middleware.BearerAuth(tokens, cfg.AuthEnabled))
				protected.Post("/calculate", calculateHandler.Handle)
			})
		})
	})

	return &Router{Handler: r, rateLimiter: rateLimiter}
}
