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

// healthPath is served without consuming a rate limit token.
const healthPath = "/api/v1/health"

// NewRouter wires middleware, routes and handlers.
//
// The order of the chain is load bearing:
//
//  1. RequestID first, so every later log line and problem document can be
//     correlated with the client's report.
//  2. BodyLimit before the logger, because http.MaxBytesReader signals an
//     oversized body to net/http through the concrete ResponseWriter; handing
//     it a wrapper would let the server drain the whole body first.
//  3. Logger outside Recoverer, so a panicking request still produces an
//     access log line rather than vanishing from 5xx alerting.
//  4. Recoverer above everything that can panic, rendering it as JSON.
//  5. SecurityHeaders before the work, so the headers are present on error
//     responses too.
//  6. CORS before the rate limiter, so a 429 still carries the headers a
//     browser needs to read it.
//  7. RateLimit last, applied globally rather than per route, so unknown
//     paths and rejected methods cannot be hammered for free.
func NewRouter(cfg *config.Config, calculator *usecase.Calculator, tokens *auth.TokenService, startedAt time.Time, logger *slog.Logger) *Router {
	rateLimiter := middleware.NewRateLimiter(
		cfg.RateLimitPerMinute, cfg.RateLimitBurst, cfg.TrustProxyHeaders, healthPath)

	calculateHandler := handler.NewCalculateHandler(calculator)
	healthHandler := handler.NewHealthHandler(startedAt, cfg.Version)
	authHandler := handler.NewAuthHandler(tokens, cfg.ClientID, cfg.ClientSecret)

	r := chi.NewRouter()
	r.Use(
		middleware.RequestID,
		middleware.BodyLimit(cfg.MaxRequestBodyBytes),
		middleware.Logger(logger),
		middleware.Recoverer(logger),
		middleware.SecurityHeaders(cfg.IsProduction(), cfg.TrustProxyHeaders),
		middleware.CORS(cfg.AllowedOrigins),
		rateLimiter.Middleware,
	)

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteProblem(w, req, domain.NewNotFoundError("The requested resource does not exist."))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteProblem(w, req, domain.NewMethodNotAllowedError(
			"The "+req.Method+" method is not supported by this resource."))
	})

	r.Route("/api/v1", func(api chi.Router) {
		// Exempted from rate limiting above, so that orchestrator probes can
		// never be throttled into reporting a healthy service as down.
		api.Get("/health", healthHandler.Handle)

		api.Post("/auth/token", authHandler.Handle)

		api.Group(func(protected chi.Router) {
			protected.Use(middleware.BearerAuth(tokens, cfg.AuthEnabled))
			protected.Post("/calculate", calculateHandler.Handle)
		})
	})

	return &Router{Handler: r, rateLimiter: rateLimiter}
}
