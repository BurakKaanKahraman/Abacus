package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/BurakKaanKahraman/abacus/backend/internal/config"
)

// Server owns the HTTP listener and its shutdown sequence.
type Server struct {
	http     *http.Server
	logger   *slog.Logger
	shutdown time.Duration
}

// New builds the HTTP server with the configured timeouts.
//
// The timeouts are not optional hardening: without them a slow or idle client
// can hold a connection, and enough of those exhaust the server.
func New(cfg *config.Config, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		http: &http.Server{
			Addr:              cfg.Address(),
			Handler:           handler,
			ReadTimeout:       cfg.ReadTimeout,
			ReadHeaderTimeout: cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			MaxHeaderBytes:    1 << 20, // 1 MiB
		},
		logger:   logger,
		shutdown: cfg.ShutdownTimeout,
	}
}

// Run serves until ctx is cancelled, then drains in-flight requests within the
// shutdown budget before returning.
func (s *Server) Run(ctx context.Context) error {
	errs := make(chan error, 1)

	go func() {
		s.logger.Info("http server listening", slog.String("address", s.http.Addr))
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("http server failed: %w", err)
			return
		}
		errs <- nil
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		s.logger.Info("shutdown signal received, draining connections",
			slog.Duration("timeout", s.shutdown))

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdown)
		defer cancel()

		if err := s.http.Shutdown(shutdownCtx); err != nil {
			// Deadline exceeded: close the remaining connections rather than
			// hanging on a client that never finishes.
			s.logger.Error("graceful shutdown timed out, forcing close",
				slog.String("error", err.Error()))
			return s.http.Close()
		}

		s.logger.Info("http server stopped cleanly")
		return nil
	}
}
