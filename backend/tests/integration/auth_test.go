package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BurakKaanKahraman/abacus/backend/internal/auth"
	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthToken_IssuesUsableToken(t *testing.T) {
	cfg := testConfig()
	cfg.AuthEnabled = true
	router := newTestRouter(t, cfg)

	recorder := do(t, router, postJSON(t, "/api/v1/auth/token", credentials(cfg)))

	require.Equal(t, http.StatusOK, recorder.Code)

	var response domain.TokenResponse
	decodeBody(t, recorder, &response)
	assert.NotEmpty(t, response.AccessToken)
	assert.Equal(t, "Bearer", response.TokenType)
	assert.Equal(t, int(cfg.JWTTTL.Seconds()), response.ExpiresIn)
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"),
		"credentials must never be cached")

	// The freshly minted token must open the protected route.
	req := postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "2+2"})
	req.Header.Set("Authorization", "Bearer "+response.AccessToken)
	assert.Equal(t, http.StatusOK, do(t, router, req).Code)
}

func TestAuthToken_ValidatesClientCredentials(t *testing.T) {
	cfg := testConfig()
	cfg.AuthEnabled = true
	cfg.ClientSecret = "configured-client-secret"
	router := newTestRouter(t, cfg)

	cases := []struct {
		name   string
		body   map[string]string
		status int
	}{
		{
			name:   "correct credentials",
			body:   map[string]string{"client_id": cfg.ClientID, "client_secret": cfg.ClientSecret},
			status: http.StatusOK,
		},
		{
			name:   "wrong secret",
			body:   map[string]string{"client_id": cfg.ClientID, "client_secret": "guess"},
			status: http.StatusUnauthorized,
		},
		{
			name:   "wrong client id",
			body:   map[string]string{"client_id": "someone-else", "client_secret": cfg.ClientSecret},
			status: http.StatusUnauthorized,
		},
		{
			name:   "missing credentials",
			body:   map[string]string{},
			status: http.StatusUnauthorized,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := do(t, router, postJSON(t, "/api/v1/auth/token", tc.body))

			require.Equal(t, tc.status, recorder.Code)
			if tc.status != http.StatusOK {
				assert.Equal(t, domain.CodeUnauthorized, decodeProblem(t, recorder).Code)
			}
		})
	}
}

// TestAuthToken_ReadsChunkedCredentials guards against gating credential
// decoding on Content-Length: a chunked request reports -1 while still
// carrying a payload.
func TestAuthToken_ReadsChunkedCredentials(t *testing.T) {
	cfg := testConfig()
	cfg.AuthEnabled = true
	router := newTestRouter(t, cfg)

	body, err := json.Marshal(credentials(cfg))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = -1 // how net/http presents a chunked body
	req.RemoteAddr = "203.0.113.10:44444"

	recorder := do(t, router, req)

	require.Equal(t, http.StatusOK, recorder.Code,
		"streamed credentials must be read, not skipped")
}

// TestAuthToken_SubjectIsAlwaysTheConfiguredClient stops a caller from
// choosing its own principal, which would otherwise appear in audit logs.
func TestAuthToken_SubjectIsAlwaysTheConfiguredClient(t *testing.T) {
	cfg := testConfig()
	cfg.AuthEnabled = true
	router := newTestRouter(t, cfg)

	body := credentials(cfg)
	body["client_id"] = cfg.ClientID // must match to authenticate at all

	recorder := do(t, router, postJSON(t, "/api/v1/auth/token", body))
	require.Equal(t, http.StatusOK, recorder.Code)

	var response domain.TokenResponse
	decodeBody(t, recorder, &response)

	claims := &jwt.RegisteredClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(response.AccessToken, claims)
	require.NoError(t, err)
	assert.Equal(t, cfg.ClientID, claims.Subject)
	assert.Equal(t, cfg.JWTIssuer, claims.Issuer)
	assert.NotEmpty(t, claims.ID, "each token must be individually traceable")
}

// TestAccessLog_RecordsAuthenticatedSubject verifies that the principal
// reaches the access log, which is written before the auth context exists.
func TestAccessLog_RecordsAuthenticatedSubject(t *testing.T) {
	cfg := testConfig()
	cfg.AuthEnabled = true
	router, logs := newTestRouterWithLogs(t, cfg)

	req := postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "1+1"})
	req.Header.Set("Authorization", "Bearer "+issueToken(t, cfg))

	require.Equal(t, http.StatusOK, do(t, router, req).Code)

	assert.Contains(t, logs.String(), `"subject":"calculator-client"`)
}

func TestProtectedRoute_RejectsInvalidCredentials(t *testing.T) {
	cfg := testConfig()
	cfg.AuthEnabled = true
	router := newTestRouter(t, cfg)

	// A token signed with a different key must never be accepted.
	foreign, _, err := auth.NewTokenService("a-completely-different-signing-secret", cfg.JWTIssuer, time.Hour).
		Issue("attacker")
	require.NoError(t, err)

	// A structurally valid token whose issuer does not match.
	wrongIssuer, _, err := auth.NewTokenService(cfg.JWTSecret, "some-other-issuer", time.Hour).
		Issue("calculator-client")
	require.NoError(t, err)

	// An expired token, minted with a lifetime already in the past.
	expired, _, err := auth.NewTokenService(cfg.JWTSecret, cfg.JWTIssuer, -time.Hour).
		Issue("calculator-client")
	require.NoError(t, err)

	// The classic algorithm confusion attempt: alg "none", no signature.
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Issuer:    cfg.JWTIssuer,
		Subject:   "attacker",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	cases := []struct {
		name   string
		header string
		detail string
	}{
		{"no header", "", "Authorization header is required"},
		{"wrong scheme", "Basic dXNlcjpwYXNz", "Bearer scheme"},
		{"empty token", "Bearer ", "must not be empty"},
		{"garbage token", "Bearer not-a-jwt", "malformed or invalid"},
		{"foreign signature", "Bearer " + foreign, "signature is invalid"},
		{"unknown issuer", "Bearer " + wrongIssuer, "unknown issuer"},
		{"expired token", "Bearer " + expired, "expired"},
		// Pinning HS256 turns the alg confusion attempt into a signature
		// failure rather than an accepted token.
		{"unsigned token", "Bearer " + unsigned, "signature is invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "1+1"})
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			recorder := do(t, router, req)

			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			assert.Contains(t, recorder.Header().Get("WWW-Authenticate"), "Bearer")

			problem := decodeProblem(t, recorder)
			assert.Equal(t, domain.CodeUnauthorized, problem.Code)
			assert.Contains(t, problem.Detail, tc.detail)
		})
	}
}

func TestProtectedRoute_AcceptsValidToken(t *testing.T) {
	cfg := testConfig()
	cfg.AuthEnabled = true
	router := newTestRouter(t, cfg)

	req := postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "10 + 20 * 3"})
	req.Header.Set("Authorization", "Bearer "+issueToken(t, cfg))

	recorder := do(t, router, req)

	require.Equal(t, http.StatusOK, recorder.Code)

	var response domain.CalculateResponse
	decodeBody(t, recorder, &response)
	assert.Equal(t, 70.0, response.Result)
}

func TestProtectedRoute_IsOpenWhenAuthDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.AuthEnabled = false
	router := newTestRouter(t, cfg)

	recorder := do(t, router, postJSON(t, "/api/v1/calculate",
		domain.CalculateRequest{Expression: "1+1"}))

	assert.Equal(t, http.StatusOK, recorder.Code)
}

// TestAuthTokenEndpoint_IsRateLimited keeps the token endpoint from being used
// as an unthrottled oracle for credential guessing.
func TestAuthTokenEndpoint_IsRateLimited(t *testing.T) {
	cfg := testConfig()
	cfg.AuthEnabled = true
	cfg.ClientSecret = "configured-client-secret"
	cfg.RateLimitBurst = 3
	router := newTestRouter(t, cfg)

	var lastStatus int
	for i := 0; i < cfg.RateLimitBurst+2; i++ {
		lastStatus = do(t, router, postJSON(t, "/api/v1/auth/token",
			map[string]string{"client_id": cfg.ClientID, "client_secret": "guess"})).Code
	}

	assert.Equal(t, http.StatusTooManyRequests, lastStatus)
}
