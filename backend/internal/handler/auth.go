package handler

import (
	"crypto/subtle"
	"net/http"

	"github.com/BurakKaanKahraman/abacus/backend/internal/auth"
	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/httpx"
)

// tokenRequest is the optional credential payload of POST /api/v1/auth/token.
type tokenRequest struct {
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// AuthHandler serves POST /api/v1/auth/token.
type AuthHandler struct {
	tokens       *auth.TokenService
	clientID     string
	clientSecret string
}

// NewAuthHandler wires the token endpoint. When clientSecret is empty the
// endpoint issues tokens without credentials, which is intended for local
// development only; production deployments must set API_CLIENT_SECRET.
func NewAuthHandler(tokens *auth.TokenService, clientID, clientSecret string) *AuthHandler {
	return &AuthHandler{tokens: tokens, clientID: clientID, clientSecret: clientSecret}
}

// Handle validates the client credentials, if any are configured, and issues a
// short-lived bearer token.
func (h *AuthHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var request tokenRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &request); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
	}

	if err := h.authenticate(request); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}

	subject := request.ClientID
	if subject == "" {
		subject = h.clientID
	}

	token, expiresIn, err := h.tokens.Issue(subject)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}

	// Tokens are credentials: keep them out of every cache.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	httpx.WriteJSON(w, http.StatusOK, domain.TokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
	})
}

// authenticate compares the supplied credentials in constant time, so that a
// wrong secret cannot be recovered by timing the response.
func (h *AuthHandler) authenticate(request tokenRequest) error {
	if h.clientSecret == "" {
		return nil
	}

	idMatches := subtle.ConstantTimeCompare([]byte(request.ClientID), []byte(h.clientID)) == 1
	secretMatches := subtle.ConstantTimeCompare([]byte(request.ClientSecret), []byte(h.clientSecret)) == 1
	if !idMatches || !secretMatches {
		return domain.NewUnauthorizedError("Invalid client credentials.")
	}
	return nil
}
