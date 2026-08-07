package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders applies defence-in-depth response headers to every request.
//
// The API only ever returns JSON, so the content security policy can be as
// restrictive as possible: nothing is loaded, framed or embedded.
//
// trustProxyHeaders decides whether X-Forwarded-Proto may be believed when
// deciding to emit HSTS. Without it, any client of a plain-HTTP server could
// send that header and pin the browser to HTTPS for a year.
func SecurityHeaders(enableHSTS, trustProxyHeaders bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			header.Set("X-Content-Type-Options", "nosniff")
			header.Set("X-Frame-Options", "DENY")
			header.Set("X-XSS-Protection", "1; mode=block")
			header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			header.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
			header.Set("Cross-Origin-Resource-Policy", "same-site")
			header.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

			// HSTS is only meaningful over TLS, and sending it from a plain
			// HTTP development server would pin browsers to https://localhost.
			if enableHSTS && isTLS(r, trustProxyHeaders) {
				header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isTLS reports whether the request reached the service over HTTPS, either
// directly or through a proxy that is trusted to report it.
func isTLS(r *http.Request, trustProxyHeaders bool) bool {
	if r.TLS != nil {
		return true
	}
	return trustProxyHeaders && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
