package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder captures the status code and response size for logging,
// since http.ResponseWriter does not expose them after the fact.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Logger emits one structured line per request.
//
// Request bodies are never logged: an expression is user input and logging it
// verbatim would put arbitrary caller-controlled text into the log stream.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			// The scope lets middleware further down report the authenticated
			// principal back to this log line.
			r, requestScope := withScope(r)

			next.ServeHTTP(recorder, r)

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", recorder.status),
				slog.Int("bytes", recorder.bytes),
				slog.Duration("duration", time.Since(started)),
				slog.String("request_id", RequestIDFrom(r.Context())),
			}
			if requestScope.subject != "" {
				attrs = append(attrs, slog.String("subject", requestScope.subject))
			}

			level := slog.LevelInfo
			switch {
			case recorder.status >= http.StatusInternalServerError:
				level = slog.LevelError
			case recorder.status >= http.StatusBadRequest:
				level = slog.LevelWarn
			}

			logger.LogAttrs(r.Context(), level, "http request", attrs...)
		})
	}
}
