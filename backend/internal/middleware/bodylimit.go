package middleware

import "net/http"

// BodyLimit caps how much of a request body the service will read.
//
// The cap is enforced by the reader itself rather than by trusting
// Content-Length, so a chunked or mislabelled body cannot exhaust memory. The
// resulting *http.MaxBytesError is translated into a 413 by the JSON decoder.
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
