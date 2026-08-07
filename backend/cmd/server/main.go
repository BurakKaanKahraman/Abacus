// Command server is the entrypoint of the calculator microservice. It loads
// the configuration, constructs the dependency graph and runs the HTTP server
// until a termination signal arrives.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BurakKaanKahraman/abacus/backend/internal/auth"
	"github.com/BurakKaanKahraman/abacus/backend/internal/config"
	"github.com/BurakKaanKahraman/abacus/backend/internal/server"
	"github.com/BurakKaanKahraman/abacus/backend/internal/usecase"
	"github.com/BurakKaanKahraman/abacus/backend/internal/usecase/parser"
)

func main() {
	if err := run(); err != nil {
		slog.Error("service terminated", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	startedAt := time.Now()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	// Dependency injection, innermost layer outwards.
	engine := parser.NewEngine(cfg.MaxExpressionLength, cfg.MaxNestingDepth)
	calculator := usecase.NewCalculator(engine)
	tokens := auth.NewTokenService(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL)

	router := server.NewRouter(cfg, calculator, tokens, startedAt, logger)
	defer func() {
		if closeErr := router.Close(); closeErr != nil {
			logger.Error("failed to release router resources", slog.String("error", closeErr.Error()))
		}
	}()

	logger.Info("starting calculator service",
		slog.String("version", cfg.Version),
		slog.String("environment", cfg.Env),
		slog.Bool("auth_enabled", cfg.AuthEnabled),
		slog.Int("rate_limit_per_minute", cfg.RateLimitPerMinute),
		slog.Any("allowed_origins", cfg.AllowedOrigins))

	if !cfg.AuthEnabled {
		logger.Warn("authentication is disabled; set AUTH_ENABLED=true and JWT_SECRET before exposing this service")
	}

	// Cancelled on SIGINT/SIGTERM, which drives the graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return server.New(cfg, router, logger).Run(ctx)
}

// newLogger builds the process logger: JSON in production for ingestion, plain
// text locally for readability.
func newLogger(cfg *config.Config) *slog.Logger {
	options := &slog.HandlerOptions{Level: slog.LevelInfo}

	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, options))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, options))
}
