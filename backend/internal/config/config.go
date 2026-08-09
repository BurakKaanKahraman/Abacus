// Package config loads and validates the application configuration from the
// environment. It is the only place that reads os.Getenv, so every other
// package receives its settings explicitly and stays testable.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment names recognised by the application.
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

const (
	// minSecretLength is the shortest HMAC secret accepted when authentication
	// is enabled. HS256 keys shorter than the hash output weaken the signature.
	minSecretLength = 32
	// minClientSecretLength is the shortest credential accepted for the token
	// endpoint when authentication is enabled.
	minClientSecretLength = 16
)

// Rate limiting defaults.
//
// The original budget was 60 requests per minute with a burst of 10, which is
// ample for a keypad. The client can now be switched to compute its live
// preview on the server, which turns a single expression into several requests
// as it is typed — at 60 a minute the preview would spend the budget and the
// calculation the user actually waited for would be refused with 429.
//
// Ten times the room keeps the preview usable while still shedding load from
// anything scripted: a client that means it can still exhaust this in seconds.
const (
	DefaultRateLimitPerMinute = 600
	DefaultRateLimitBurst     = 30
)

// Config is the fully resolved application configuration.
type Config struct {
	Env     string
	Version string
	Port    int

	// AllowedOrigins is the CORS allowlist. Requests from any other origin are
	// answered without CORS headers, so browsers reject them.
	AllowedOrigins []string
	// TrustProxyHeaders enables reading the client IP from X-Forwarded-For.
	// It defaults to false: when the service is exposed directly, an attacker
	// could otherwise spoof the header and bypass per-IP rate limiting.
	TrustProxyHeaders bool

	RateLimitPerMinute int
	RateLimitBurst     int

	AuthEnabled  bool
	JWTSecret    string
	JWTIssuer    string
	JWTTTL       time.Duration
	ClientID     string
	ClientSecret string

	MaxExpressionLength int
	MaxNestingDepth     int
	MaxRequestBodyBytes int64

	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// Load reads the configuration from the environment, applies defaults and
// validates the result. It never returns a partially valid Config.
func Load() (*Config, error) {
	l := &loader{}

	cfg := &Config{
		Env:     l.str("APP_ENV", EnvDevelopment),
		Version: l.str("APP_VERSION", "1.0.0"),
		Port:    l.int("PORT", 8080),

		AllowedOrigins:    l.slice("ALLOWED_ORIGINS", []string{"http://localhost:5173", "http://localhost:3000"}),
		TrustProxyHeaders: l.boolean("TRUST_PROXY_HEADERS", false),

		RateLimitPerMinute: l.int("RATE_LIMIT_PER_MINUTE", DefaultRateLimitPerMinute),
		RateLimitBurst:     l.int("RATE_LIMIT_BURST", DefaultRateLimitBurst),

		AuthEnabled:  l.boolean("AUTH_ENABLED", false),
		JWTSecret:    l.str("JWT_SECRET", ""),
		JWTIssuer:    l.str("JWT_ISSUER", "abacus-calculator"),
		JWTTTL:       time.Duration(l.int("JWT_TTL_SECONDS", 3600)) * time.Second,
		ClientID:     l.str("API_CLIENT_ID", "calculator-client"),
		ClientSecret: l.str("API_CLIENT_SECRET", ""),

		MaxExpressionLength: l.int("MAX_EXPRESSION_LENGTH", 500),
		MaxNestingDepth:     l.int("MAX_NESTING_DEPTH", 20),
		MaxRequestBodyBytes: int64(l.int("MAX_REQUEST_BODY_BYTES", 64*1024)),

		ReadTimeout:     time.Duration(l.int("READ_TIMEOUT_SECONDS", 5)) * time.Second,
		WriteTimeout:    time.Duration(l.int("WRITE_TIMEOUT_SECONDS", 10)) * time.Second,
		IdleTimeout:     time.Duration(l.int("IDLE_TIMEOUT_SECONDS", 60)) * time.Second,
		ShutdownTimeout: time.Duration(l.int("SHUTDOWN_TIMEOUT_SECONDS", 10)) * time.Second,
	}

	if err := l.err(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// IsProduction reports whether the service runs in production mode, which
// switches on HSTS and JSON logging.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.Env, EnvProduction)
}

// Address is the listen address for the HTTP server.
func (c *Config) Address() string {
	return fmt.Sprintf(":%d", c.Port)
}

// validate rejects configurations that would start an insecure or unusable
// server. Failing at startup is preferable to failing per request.
func (c *Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("config: PORT must be between 1 and 65535, got %d", c.Port)
	}
	if c.RateLimitPerMinute < 1 {
		return fmt.Errorf("config: RATE_LIMIT_PER_MINUTE must be positive, got %d", c.RateLimitPerMinute)
	}
	if c.RateLimitBurst < 1 {
		return fmt.Errorf("config: RATE_LIMIT_BURST must be positive, got %d", c.RateLimitBurst)
	}
	if c.MaxExpressionLength < 1 {
		return fmt.Errorf("config: MAX_EXPRESSION_LENGTH must be positive, got %d", c.MaxExpressionLength)
	}
	if c.MaxNestingDepth < 1 {
		return fmt.Errorf("config: MAX_NESTING_DEPTH must be positive, got %d", c.MaxNestingDepth)
	}
	if c.MaxRequestBodyBytes < 1 {
		return fmt.Errorf("config: MAX_REQUEST_BODY_BYTES must be positive, got %d", c.MaxRequestBodyBytes)
	}
	if c.JWTTTL <= 0 {
		return fmt.Errorf("config: JWT_TTL_SECONDS must be positive")
	}
	if len(c.AllowedOrigins) == 0 {
		return fmt.Errorf("config: ALLOWED_ORIGINS must list at least one origin")
	}
	if c.AuthEnabled && len(c.JWTSecret) < minSecretLength {
		return fmt.Errorf("config: JWT_SECRET must be at least %d characters when AUTH_ENABLED=true", minSecretLength)
	}
	// Without a client secret the token endpoint would hand a valid bearer
	// token to any anonymous caller, which makes the whole auth layer
	// decorative. Enabling authentication must gate the credential too.
	if c.AuthEnabled && len(c.ClientSecret) < minClientSecretLength {
		return fmt.Errorf(
			"config: API_CLIENT_SECRET must be at least %d characters when AUTH_ENABLED=true, "+
				"otherwise /api/v1/auth/token issues tokens to anonymous callers",
			minClientSecretLength)
	}
	return nil
}

// loader reads environment variables and accumulates parse failures, so that a
// typo surfaces as a startup error instead of silently reverting to a default.
type loader struct {
	errs []string
}

// err reports every malformed variable at once, rather than one per restart.
func (l *loader) err() error {
	if len(l.errs) == 0 {
		return nil
	}
	return fmt.Errorf("config: %s", strings.Join(l.errs, "; "))
}

func (l *loader) invalid(key, value, expected string) {
	l.errs = append(l.errs, fmt.Sprintf("%s=%q is not a valid %s", key, value, expected))
}

func (l *loader) str(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func (l *loader) int(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		l.invalid(key, value, "integer")
		return fallback
	}
	return parsed
}

func (l *loader) boolean(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		l.invalid(key, value, "boolean")
		return fallback
	}
	return parsed
}

// slice reads a comma separated list, dropping empty entries.
func (l *loader) slice(key string, fallback []string) []string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}
