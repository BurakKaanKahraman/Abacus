// Package httpx renders HTTP responses. It is shared by the handler and
// middleware layers so that a rate limit rejection and a division-by-zero
// error are serialised by exactly the same code path, and every error the API
// emits is a valid RFC 7807 problem document.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
)

// ProblemContentType is the media type mandated by RFC 7807 for error
// documents. Using it rather than application/json lets clients recognise a
// problem response without inspecting the status code.
const ProblemContentType = "application/problem+json"

// WriteJSON renders a successful response.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// The status line is already on the wire, so the response cannot be
		// rewritten; record it and let the client see a truncated body.
		slog.Error("failed to encode response body", slog.String("error", err.Error()))
	}
}

// WriteProblem renders any error as an RFC 7807 problem document. Errors that
// are not *domain.AppError become a generic 500, so internal details never
// reach the client.
func WriteProblem(w http.ResponseWriter, r *http.Request, err error) {
	appErr, ok := domain.AsAppError(err)
	if !ok {
		slog.Error("unhandled error",
			slog.String("error", err.Error()),
			slog.String("path", r.URL.Path))
		appErr = domain.NewInternalError("An unexpected error occurred while processing the request.")
	}

	w.Header().Set("Content-Type", ProblemContentType)
	w.WriteHeader(appErr.Status)

	problem := domain.ProblemDetails{
		Type:      appErr.Type,
		Title:     appErr.Title,
		Status:    appErr.Status,
		Detail:    appErr.Detail,
		Code:      appErr.Code,
		Instance:  r.URL.Path,
		Timestamp: Now(),
	}
	if encodeErr := json.NewEncoder(w).Encode(problem); encodeErr != nil {
		slog.Error("failed to encode problem document", slog.String("error", encodeErr.Error()))
	}
}

// Now returns the current UTC timestamp in the format used across the API.
func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
