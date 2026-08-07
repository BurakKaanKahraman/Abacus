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

// minSecretLength is the shortest HMAC secret accepted when authentication is
// enabled. HS256 keys shorter than the hash output weaken the signature.
const minSecretLength = 32

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
	cfg := &Config{
		Env:     envString("APP_ENV", EnvDevelopment),
		Version: envString("APP_VERSION", "1.0.0"),
		Port:    envInt("PORT", 8080),

		AllowedOrigins:    envStringSlice("ALLOWED_ORIGINS", []string{"http://localhost:5173", "http://localhost:3000"}),
		TrustProxyHeaders: envBool("TRUST_PROXY_HEADERS", false),

		RateLimitPerMinute: envInt("RATE_LIMIT_PER_MINUTE", 60),
		RateLimitBurst:     envInt("RATE_LIMIT_BURST", 10),

		AuthEnabled:  envBool("AUTH_ENABLED", false),
		JWTSecret:    envString("JWT_SECRET", ""),
		JWTIssuer:    envString("JWT_ISSUER", "abacus-calculator"),
		JWTTTL:       time.Duration(envInt("JWT_TTL_SECONDS", 3600)) * time.Second,
		ClientID:     envString("API_CLIENT_ID", "calculator-client"),
		ClientSecret: envString("API_CLIENT_SECRET", ""),

		MaxExpressionLength: envInt("MAX_EXPRESSION_LENGTH", 500),
		MaxNestingDepth:     envInt("MAX_NESTING_DEPTH", 20),
		MaxRequestBodyBytes: int64(envInt("MAX_REQUEST_BODY_BYTES", 64*1024)),

		ReadTimeout:     time.Duration(envInt("READ_TIMEOUT_SECONDS", 5)) * time.Second,
		WriteTimeout:    time.Duration(envInt("WRITE_TIMEOUT_SECONDS", 10)) * time.Second,
		IdleTimeout:     time.Duration(envInt("IDLE_TIMEOUT_SECONDS", 60)) * time.Second,
		ShutdownTimeout: time.Duration(envInt("SHUTDOWN_TIMEOUT_SECONDS", 10)) * time.Second,
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
	return nil
}

func envString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

// envStringSlice reads a comma separated list, dropping empty entries.
func envStringSlice(key string, fallback []string) []string {
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
