package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// corsMaxAge is how long a browser may cache the preflight result.
const corsMaxAge = 10 * time.Minute

// CORS answers cross-origin requests for the configured allowlist only.
//
// The allowlist is matched exactly and the origin is echoed back rather than
// answered with `*`, which keeps the policy compatible with credentialed
// requests and prevents an arbitrary site from reading API responses.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	allowAny := false
	for _, origin := range allowedOrigins {
		if origin == "*" {
			allowAny = true
		}
		allowed[strings.ToLower(strings.TrimRight(origin, "/"))] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				// Not a browser cross-origin request; nothing to negotiate.
				next.ServeHTTP(w, r)
				return
			}

			// Responses vary per origin, so caches must not reuse one origin's
			// response for another.
			w.Header().Add("Vary", "Origin")

			if !isOriginAllowed(origin, allowed, allowAny) {
				// Deliberately no CORS headers: the browser blocks the read.
				// Preflights are terminated to avoid leaking route existence.
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			header := w.Header()
			header.Set("Access-Control-Allow-Origin", origin)
			header.Set("Access-Control-Expose-Headers", strings.Join([]string{
				RequestIDHeader, "Retry-After", "X-RateLimit-Limit", "X-RateLimit-Remaining",
			}, ", "))

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				header.Add("Vary", "Access-Control-Request-Method")
				header.Add("Vary", "Access-Control-Request-Headers")
				header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				header.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, "+RequestIDHeader)
				header.Set("Access-Control-Max-Age", strconv.Itoa(int(corsMaxAge.Seconds())))
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isOriginAllowed(origin string, allowed map[string]struct{}, allowAny bool) bool {
	if allowAny {
		return true
	}
	_, ok := allowed[strings.ToLower(strings.TrimRight(origin, "/"))]
	return ok
}
