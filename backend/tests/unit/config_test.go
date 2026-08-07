package unit

import (
	"testing"
	"time"

	"github.com/BurakKaanKahraman/abacus/backend/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests for configuration loading. The environment is mutated through
// t.Setenv, which restores the previous value automatically.

func TestConfigLoad_AppliesDefaults(t *testing.T) {
	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, config.EnvDevelopment, cfg.Env)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, ":8080", cfg.Address())
	assert.Equal(t, 60, cfg.RateLimitPerMinute)
	assert.Equal(t, 10, cfg.RateLimitBurst)
	assert.Equal(t, 500, cfg.MaxExpressionLength)
	assert.Equal(t, 20, cfg.MaxNestingDepth)
	assert.Equal(t, time.Hour, cfg.JWTTTL)
	assert.False(t, cfg.AuthEnabled, "authentication is opt-in")
	assert.False(t, cfg.TrustProxyHeaders, "proxy headers are untrusted unless declared")
	assert.False(t, cfg.IsProduction())
	assert.NotEmpty(t, cfg.AllowedOrigins)
}

func TestConfigLoad_ReadsEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORT", "9090")
	t.Setenv("ALLOWED_ORIGINS", "https://app.example.com, https://admin.example.com ,")
	t.Setenv("RATE_LIMIT_PER_MINUTE", "120")
	t.Setenv("TRUST_PROXY_HEADERS", "true")
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("JWT_SECRET", "a-sufficiently-long-secret-for-hs256!")
	t.Setenv("JWT_TTL_SECONDS", "900")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.True(t, cfg.IsProduction())
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, []string{"https://app.example.com", "https://admin.example.com"}, cfg.AllowedOrigins,
		"blank entries must be dropped and values trimmed")
	assert.Equal(t, 120, cfg.RateLimitPerMinute)
	assert.True(t, cfg.TrustProxyHeaders)
	assert.True(t, cfg.AuthEnabled)
	assert.Equal(t, 15*time.Minute, cfg.JWTTTL)
}

func TestConfigLoad_FallsBackOnUnparsableValues(t *testing.T) {
	t.Setenv("PORT", "not-a-number")
	t.Setenv("AUTH_ENABLED", "maybe")
	t.Setenv("ALLOWED_ORIGINS", "  ,  ")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Port)
	assert.False(t, cfg.AuthEnabled)
	assert.NotEmpty(t, cfg.AllowedOrigins)
}

// TestConfigLoad_RejectsInsecureAndUnusableSettings pins the startup contract:
// a misconfigured service must fail immediately rather than per request.
func TestConfigLoad_RejectsInsecureAndUnusableSettings(t *testing.T) {
	cases := []struct {
		name   string
		env    map[string]string
		detail string
	}{
		{
			name:   "auth enabled without a secret",
			env:    map[string]string{"AUTH_ENABLED": "true"},
			detail: "JWT_SECRET",
		},
		{
			name:   "auth enabled with a short secret",
			env:    map[string]string{"AUTH_ENABLED": "true", "JWT_SECRET": "too-short"},
			detail: "JWT_SECRET",
		},
		{
			name:   "port out of range",
			env:    map[string]string{"PORT": "70000"},
			detail: "PORT",
		},
		{
			name:   "non-positive rate limit",
			env:    map[string]string{"RATE_LIMIT_PER_MINUTE": "0"},
			detail: "RATE_LIMIT_PER_MINUTE",
		},
		{
			name:   "non-positive burst",
			env:    map[string]string{"RATE_LIMIT_BURST": "-1"},
			detail: "RATE_LIMIT_BURST",
		},
		{
			name:   "non-positive body limit",
			env:    map[string]string{"MAX_REQUEST_BODY_BYTES": "0"},
			detail: "MAX_REQUEST_BODY_BYTES",
		},
		{
			name:   "non-positive token lifetime",
			env:    map[string]string{"JWT_TTL_SECONDS": "0"},
			detail: "JWT_TTL_SECONDS",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			_, err := config.Load()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.detail)
		})
	}
}
