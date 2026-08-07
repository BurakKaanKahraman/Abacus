// Package integration exercises the HTTP delivery layer through the real
// router, so every assertion runs against the complete middleware chain rather
// than a handler in isolation.
package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BurakKaanKahraman/abacus/backend/internal/auth"
	"github.com/BurakKaanKahraman/abacus/backend/internal/config"
	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/server"
	"github.com/BurakKaanKahraman/abacus/backend/internal/usecase"
	"github.com/BurakKaanKahraman/abacus/backend/internal/usecase/parser"
	"github.com/stretchr/testify/require"
)

// testSecret is a throwaway HMAC key that satisfies the 32 character minimum.
const testSecret = "integration-test-secret-key-0123456789"

// testConfig returns a configuration with production-like defaults that
// individual tests can adjust.
func testConfig() *config.Config {
	return &config.Config{
		Env:                 config.EnvDevelopment,
		Version:             "1.0.0-test",
		Port:                8080,
		AllowedOrigins:      []string{"http://localhost:5173"},
		RateLimitPerMinute:  60,
		RateLimitBurst:      10,
		AuthEnabled:         false,
		JWTSecret:           testSecret,
		JWTIssuer:           "abacus-calculator",
		JWTTTL:              time.Hour,
		ClientID:            "calculator-client",
		MaxExpressionLength: parser.DefaultMaxExpressionLength,
		MaxNestingDepth:     parser.DefaultMaxNestingDepth,
		MaxRequestBodyBytes: 64 * 1024,
		ReadTimeout:         5 * time.Second,
		WriteTimeout:        10 * time.Second,
		IdleTimeout:         60 * time.Second,
		ShutdownTimeout:     10 * time.Second,
	}
}

// newTestRouter builds the real router and registers its cleanup.
func newTestRouter(t *testing.T, cfg *config.Config) *server.Router {
	t.Helper()

	engine := parser.NewEngine(cfg.MaxExpressionLength, cfg.MaxNestingDepth)
	calculator := usecase.NewCalculator(engine)
	tokens := auth.NewTokenService(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	router := server.NewRouter(cfg, calculator, tokens, time.Now(), logger)
	t.Cleanup(func() { require.NoError(t, router.Close()) })
	return router
}

// do executes a request against the router and returns the recorded response.
func do(t *testing.T, router http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

// postJSON builds a JSON POST request. A raw string body is sent verbatim so
// that malformed payloads can be exercised.
func postJSON(t *testing.T, path string, body any) *http.Request {
	t.Helper()

	var payload []byte
	switch typed := body.(type) {
	case nil:
		payload = nil
	case string:
		payload = []byte(typed)
	default:
		encoded, err := json.Marshal(typed)
		require.NoError(t, err)
		payload = encoded
	}

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	// httptest defaults to 192.0.2.1:1234; an explicit address keeps per-IP
	// rate limiting isolated between tests that need it.
	req.RemoteAddr = "203.0.113.10:44444"
	return req
}

// decodeBody unmarshals a response body into dst.
func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder, dst any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), dst))
}

// decodeProblem unmarshals an RFC 7807 document and asserts its shape.
func decodeProblem(t *testing.T, recorder *httptest.ResponseRecorder) domain.ProblemDetails {
	t.Helper()

	var problem domain.ProblemDetails
	decodeBody(t, recorder, &problem)

	require.Equal(t, "application/problem+json", recorder.Header().Get("Content-Type"),
		"errors must be served as RFC 7807 problem documents")
	require.Equal(t, recorder.Code, problem.Status, "problem status must match the HTTP status")
	require.NotEmpty(t, problem.Type)
	require.NotEmpty(t, problem.Title)
	require.NotEmpty(t, problem.Code)
	require.NotEmpty(t, problem.Timestamp)
	return problem
}

// issueToken mints a valid bearer token for the test configuration.
func issueToken(t *testing.T, cfg *config.Config) string {
	t.Helper()
	token, _, err := auth.NewTokenService(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL).Issue("calculator-client")
	require.NoError(t, err)
	return token
}
