package integration

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityHeaders_ArePresentOnEveryResponse(t *testing.T) {
	router := newTestRouter(t, testConfig())

	expected := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}

	requests := map[string]*http.Request{
		"success": postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "1+1"}),
		"error":   postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "1/0"}),
		"health":  httptest.NewRequest(http.MethodGet, "/api/v1/health", nil),
		"unknown": httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil),
	}

	for name, req := range requests {
		t.Run(name, func(t *testing.T) {
			recorder := do(t, router, req)

			for header, value := range expected {
				assert.Equal(t, value, recorder.Header().Get(header), "missing %s", header)
			}
			assert.Contains(t, recorder.Header().Get("Content-Security-Policy"), "default-src 'none'")
			assert.NotEmpty(t, recorder.Header().Get("X-Request-ID"))
		})
	}
}

// TestSecurityHeaders_HSTSOnlyOverTLS keeps a development server from pinning
// browsers to https://localhost.
func TestSecurityHeaders_HSTSOnlyOverTLS(t *testing.T) {
	cfg := testConfig()
	cfg.Env = "production"
	router := newTestRouter(t, cfg)

	plain := do(t, router, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	assert.Empty(t, plain.Header().Get("Strict-Transport-Security"))

	forwarded := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	forwarded.Header.Set("X-Forwarded-Proto", "https")
	secure := do(t, router, forwarded)
	assert.Contains(t, secure.Header().Get("Strict-Transport-Security"), "max-age=31536000")
}

func TestRequestID_IsEchoedAndSanitized(t *testing.T) {
	router := newTestRouter(t, testConfig())

	t.Run("clean identifier is echoed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		req.Header.Set("X-Request-ID", "trace-abc-123")

		recorder := do(t, router, req)

		assert.Equal(t, "trace-abc-123", recorder.Header().Get("X-Request-ID"))
	})

	t.Run("header injection attempt is replaced", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		req.Header.Set("X-Request-ID", "abc<script>alert(1)</script>")

		recorder := do(t, router, req)

		echoed := recorder.Header().Get("X-Request-ID")
		assert.NotContains(t, echoed, "<script>")
		assert.NotEmpty(t, echoed, "a generated identifier must replace the rejected one")
	})
}

func TestCORS_AllowsConfiguredOriginOnly(t *testing.T) {
	router := newTestRouter(t, testConfig())

	t.Run("allowed origin is echoed", func(t *testing.T) {
		req := postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "1+1"})
		req.Header.Set("Origin", "http://localhost:5173")

		recorder := do(t, router, req)

		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "http://localhost:5173", recorder.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, recorder.Header().Values("Vary"), "Origin")
	})

	t.Run("unknown origin receives no CORS headers", func(t *testing.T) {
		req := postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "1+1"})
		req.Header.Set("Origin", "https://evil.example.com")

		recorder := do(t, router, req)

		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"),
			"a browser must refuse to expose this response")
	})

	t.Run("preflight from allowed origin succeeds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/calculate", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)

		recorder := do(t, router, req)

		require.Equal(t, http.StatusNoContent, recorder.Code)
		assert.Contains(t, recorder.Header().Get("Access-Control-Allow-Methods"), http.MethodPost)
		assert.Contains(t, recorder.Header().Get("Access-Control-Allow-Headers"), "Authorization")
		assert.NotEmpty(t, recorder.Header().Get("Access-Control-Max-Age"))
	})

	t.Run("preflight from unknown origin is forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/calculate", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)

		recorder := do(t, router, req)

		assert.Equal(t, http.StatusForbidden, recorder.Code)
	})
}

func TestRateLimit_ThrottlesBurstAndReportsRetryAfter(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimitPerMinute = 60
	cfg.RateLimitBurst = 5
	router := newTestRouter(t, cfg)

	var lastRecorder *httptest.ResponseRecorder
	allowed := 0
	throttled := 0

	// The bucket refills at one token per second, so a tight loop of
	// burst + 5 requests cannot be replenished mid-flight.
	for i := 0; i < cfg.RateLimitBurst+5; i++ {
		lastRecorder = do(t, router, postJSON(t, "/api/v1/calculate",
			domain.CalculateRequest{Expression: "1+1"}))
		if lastRecorder.Code == http.StatusTooManyRequests {
			throttled++
		} else {
			allowed++
		}
	}

	assert.Equal(t, cfg.RateLimitBurst, allowed, "the burst budget must be honoured exactly")
	assert.Positive(t, throttled, "requests beyond the burst must be throttled")

	require.Equal(t, http.StatusTooManyRequests, lastRecorder.Code)
	problem := decodeProblem(t, lastRecorder)
	assert.Equal(t, domain.CodeRateLimitExceeded, problem.Code)
	assert.Contains(t, problem.Detail, "60 requests per minute")
	assert.NotEmpty(t, lastRecorder.Header().Get("Retry-After"))
	assert.Equal(t, "60", lastRecorder.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "0", lastRecorder.Header().Get("X-RateLimit-Remaining"))
}

func TestRateLimit_IsPerClientAddress(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimitBurst = 2
	router := newTestRouter(t, cfg)

	exhaust := func(remoteAddr string) int {
		var status int
		for i := 0; i < cfg.RateLimitBurst+1; i++ {
			req := postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "1+1"})
			req.RemoteAddr = remoteAddr
			status = do(t, router, req).Code
		}
		return status
	}

	require.Equal(t, http.StatusTooManyRequests, exhaust("198.51.100.1:1111"))

	// A different client must start with a full bucket.
	req := postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "1+1"})
	req.RemoteAddr = "198.51.100.2:2222"
	assert.Equal(t, http.StatusOK, do(t, router, req).Code)
}

// TestRateLimit_IgnoresSpoofedForwardedHeader guards the default deployment:
// with TrustProxyHeaders disabled, a client cannot mint a fresh bucket per
// request by varying X-Forwarded-For.
func TestRateLimit_IgnoresSpoofedForwardedHeader(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimitBurst = 2
	cfg.TrustProxyHeaders = false
	router := newTestRouter(t, cfg)

	var lastStatus int
	for i := 0; i < cfg.RateLimitBurst+1; i++ {
		req := postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "1+1"})
		req.RemoteAddr = "198.51.100.9:3333"
		req.Header.Set("X-Forwarded-For", "10.0.0."+string(rune('1'+i)))
		lastStatus = do(t, router, req).Code
	}

	assert.Equal(t, http.StatusTooManyRequests, lastStatus)
}

func TestHealth_IsExemptFromRateLimiting(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimitBurst = 2
	router := newTestRouter(t, cfg)

	for i := 0; i < 10; i++ {
		recorder := do(t, router, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
		require.Equal(t, http.StatusOK, recorder.Code, "probe %d must not be throttled", i)
	}
}

func TestRouting_UnknownRoutesReturnProblemDocuments(t *testing.T) {
	router := newTestRouter(t, testConfig())

	t.Run("unknown path", func(t *testing.T) {
		recorder := do(t, router, httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil))

		require.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Equal(t, domain.CodeNotFound, decodeProblem(t, recorder).Code)
	})

	t.Run("wrong method", func(t *testing.T) {
		recorder := do(t, router, httptest.NewRequest(http.MethodGet, "/api/v1/calculate", nil))

		require.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
		assert.Equal(t, domain.CodeMethodNotAllowed, decodeProblem(t, recorder).Code)
	})
}

// TestRecoverer_TurnsPanicsIntoProblemDocuments guards the last line of
// defence: a panic must become a 500 JSON response, and the stack trace must
// never reach the client.
func TestRecoverer_TurnsPanicsIntoProblemDocuments(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom: internal detail that must not leak")
	})
	handler := middleware.RequestID(middleware.Recoverer(logger)(panicking))

	recorder := do(t, handler, httptest.NewRequest(http.MethodGet, "/api/v1/calculate", nil))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	problem := decodeProblem(t, recorder)
	assert.Equal(t, domain.CodeInternal, problem.Code)
	assert.NotContains(t, recorder.Body.String(), "boom")
	assert.NotContains(t, recorder.Body.String(), "goroutine")
}

// TestRateLimit_HonoursForwardedHeaderWhenTrusted is the counterpart to the
// spoofing test: behind a declared proxy the real client must be distinguished.
func TestRateLimit_HonoursForwardedHeaderWhenTrusted(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimitBurst = 2
	cfg.TrustProxyHeaders = true
	router := newTestRouter(t, cfg)

	send := func(forwardedFor string) int {
		req := postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "1+1"})
		req.RemoteAddr = "10.0.0.1:9999" // the proxy itself
		req.Header.Set("X-Forwarded-For", forwardedFor)
		return do(t, router, req).Code
	}

	var last int
	for i := 0; i < cfg.RateLimitBurst+1; i++ {
		last = send("198.51.100.20")
	}
	require.Equal(t, http.StatusTooManyRequests, last, "the forwarded client must be throttled")

	assert.Equal(t, http.StatusOK, send("198.51.100.21"),
		"a different forwarded client must have its own bucket")
}

func TestCORS_WildcardOriginIsSupported(t *testing.T) {
	cfg := testConfig()
	cfg.AllowedOrigins = []string{"*"}
	router := newTestRouter(t, cfg)

	req := postJSON(t, "/api/v1/calculate", domain.CalculateRequest{Expression: "1+1"})
	req.Header.Set("Origin", "https://any.example.com")

	recorder := do(t, router, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "https://any.example.com", recorder.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_SameOriginRequestsPassThroughUntouched(t *testing.T) {
	router := newTestRouter(t, testConfig())

	// No Origin header: a cURL or server-to-server call.
	recorder := do(t, router, postJSON(t, "/api/v1/calculate",
		domain.CalculateRequest{Expression: "1+1"}))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
}

func TestHealth_ReportsStatusAndVersion(t *testing.T) {
	cfg := testConfig()
	router := newTestRouter(t, cfg)

	recorder := do(t, router, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	require.Equal(t, http.StatusOK, recorder.Code)

	var response domain.HealthResponse
	decodeBody(t, recorder, &response)
	assert.Equal(t, "UP", response.Status)
	assert.Equal(t, cfg.Version, response.Version)
	assert.NotEmpty(t, response.Uptime)
}
