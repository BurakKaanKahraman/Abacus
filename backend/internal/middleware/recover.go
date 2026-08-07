package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/httpx"
)

// Recoverer converts a panic in any downstream handler into a 500 problem
// document instead of letting it tear down the connection.
//
// The stack trace is logged, never returned: it would disclose internal paths
// and code structure to the caller.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				// A hijacked or aborted connection cannot be written to, so
				// this one is re-panicked for net/http to handle. Matched
				// with errors.Is rather than ==, so a wrapped ErrAbortHandler
				// is still recognised.
				if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(recovered)
				}

				logger.Error("recovered from panic",
					slog.Any("panic", recovered),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("request_id", RequestIDFrom(r.Context())),
					slog.String("stack", string(debug.Stack())))

				httpx.WriteProblem(w, r, domain.NewInternalError(
					"An unexpected error occurred while processing the request."))
			}()

			next.ServeHTTP(w, r)
		})
	}
}
