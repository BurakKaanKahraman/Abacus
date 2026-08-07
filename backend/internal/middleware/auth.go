package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/httpx"
)

// TokenVerifier is the contract the auth middleware needs from the token
// service. Declaring it here keeps the middleware testable with a stub and
// independent of the JWT library.
type TokenVerifier interface {
	Verify(token string) (subject string, err error)
}

// BearerAuth validates the Authorization header when authentication is
// enabled. With enabled=false the middleware is a pass-through, which is how
// the service runs in local development.
//
// The WWW-Authenticate header is set on rejection so that clients learn the
// expected scheme, as required by RFC 6750.
func BearerAuth(verifier TokenVerifier, enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}

			token, err := bearerToken(r)
			if err != nil {
				reject(w, r, err)
				return
			}

			subject, err := verifier.Verify(token)
			if err != nil {
				reject(w, r, err)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), subjectKey, subject)))
		})
	}
}

// bearerToken extracts the credential from the Authorization header.
func bearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", domain.NewUnauthorizedError("Authorization header is required.")
	}

	scheme, credentials, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", domain.NewUnauthorizedError("Authorization header must use the Bearer scheme.")
	}

	credentials = strings.TrimSpace(credentials)
	if credentials == "" {
		return "", domain.NewUnauthorizedError("Bearer token must not be empty.")
	}
	return credentials, nil
}

func reject(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="abacus-calculator"`)
	httpx.WriteProblem(w, r, err)
}
