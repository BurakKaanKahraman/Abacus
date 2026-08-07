// Package middleware holds the cross-cutting HTTP concerns: request
// identity, logging, panic recovery, security headers, CORS, rate limiting
// and bearer token authentication.
//
// Every middleware is an ordinary func(http.Handler) http.Handler, so the
// chain can be assembled and tested without a framework.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
)

// contextKey is unexported so that no other package can collide with these
// keys or overwrite the values they guard.
type contextKey string

const (
	requestIDKey contextKey = "request_id"
	subjectKey   contextKey = "auth_subject"
	scopeKey     contextKey = "request_scope"
)

// scope carries values that middleware discovers mid-chain but that outer
// middleware needs afterwards.
//
// A context value cannot serve this purpose on its own: a handler deriving a
// new context does so below the logger, which still holds the original. The
// logger therefore installs a pointer that inner middleware can fill in.
// Only one goroutine touches a request, so no locking is required.
type scope struct {
	subject string
}

// withScope installs a fresh scope for the request.
func withScope(r *http.Request) (*http.Request, *scope) {
	s := &scope{}
	return r.WithContext(context.WithValue(r.Context(), scopeKey, s)), s
}

// scopeFrom returns the request scope, or nil when none was installed.
func scopeFrom(ctx context.Context) *scope {
	s, _ := ctx.Value(scopeKey).(*scope)
	return s
}

// RequestIDHeader carries the correlation identifier in and out of the service.
const RequestIDHeader = "X-Request-ID"

// RequestIDFrom returns the correlation identifier attached to the request.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// SubjectFrom returns the authenticated subject, empty when authentication is
// disabled or the route is public.
func SubjectFrom(ctx context.Context) string {
	subject, _ := ctx.Value(subjectKey).(string)
	return subject
}

// RequestID attaches a correlation identifier to every request, reusing a
// caller supplied one when it looks safe to echo back.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitizeRequestID(r.Header.Get(RequestIDHeader))
		if id == "" {
			id = newRequestID()
		}

		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// sanitizeRequestID accepts only short alphanumeric identifiers. An arbitrary
// client string is never echoed into a response header, which would allow
// header injection and log forging.
func sanitizeRequestID(value string) string {
	const maxLen = 64
	if value == "" || len(value) > maxLen {
		return ""
	}
	for _, r := range value {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_'
		if !isAllowed {
			return ""
		}
	}
	return value
}

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(buf)
}

// ClientIP resolves the address used for per-client rate limiting.
//
// X-Forwarded-For is only consulted when the deployment is known to sit behind
// a trusted proxy: otherwise any client could spoof the header and be granted
// a fresh rate limit bucket per request.
func ClientIP(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			if first := strings.TrimSpace(strings.Split(forwarded, ",")[0]); first != "" {
				return first
			}
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
