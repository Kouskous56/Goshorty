package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds application configuration
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Security SecurityConfig
	Ops      OpsConfig
	TTL      TTLConfig
}

// ServerConfig holds server settings
type ServerConfig struct {
	Port    string
	BaseURL string // e.g., "http://localhost:8080"
}

// DatabaseConfig holds persistent storage settings.
type DatabaseConfig struct {
	URL string
}

// SecurityConfig holds browser, reverse-proxy trust, and token settings.
type SecurityConfig struct {
	AllowedOrigins       []string
	TrustedProxies       []string
	MaxRequestBytes      int64
	MetricsToken         string
	TokenTTL             time.Duration
	TokenIssuer          string
	TokenAudience        string
	RegisterLimitPerHour int
}

// TTLConfig holds TTL duration settings
type TTLConfig struct {
	Options map[string]time.Duration
}

// OpsConfig holds operational and debugging surface settings.
type OpsConfig struct {
	// PprofEnabled opts the Go runtime profiling endpoints (/debug/pprof/*)
	// in. These endpoints expose runtime internals and must never be reachable
	// from the public internet, so they stay off unless explicitly enabled
	// with PPROF_ENABLED=true.
	PprofEnabled bool
}

func portFromEnv(defaultPort string) string {
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return defaultPort
}

func baseURLFromEnv(defaultURL string) string {
	if u := os.Getenv("PUBLIC_BASE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	// BASE_URL remains supported for backward compatibility.
	if u := os.Getenv("BASE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	if p := os.Getenv("PORT"); p != "" {
		return "http://localhost:" + p
	}
	return strings.TrimRight(defaultURL, "/")
}

// NewConfig creates a new configuration with default values
func NewConfig() *Config {
	baseURL := baseURLFromEnv("http://localhost:8080")
	return &Config{
		Server: ServerConfig{
			Port:    portFromEnv(":8080"),
			BaseURL: baseURL,
		},
		Database: DatabaseConfig{
			URL: os.Getenv("DATABASE_URL"),
		},
		Security: SecurityConfig{
			AllowedOrigins:       listFromEnv("ALLOWED_ORIGINS", baseURL),
			TrustedProxies:       listFromEnv("TRUSTED_PROXIES", "100.64.0.0/10"),
			MaxRequestBytes:      1 << 20,
			MetricsToken:         os.Getenv("METRICS_TOKEN"),
			TokenTTL:             tokenTTLFromEnv(),
			TokenIssuer:          os.Getenv("TOKEN_ISSUER"),
			TokenAudience:        os.Getenv("TOKEN_AUDIENCE"),
			RegisterLimitPerHour: intFromEnv("REGISTER_LIMIT_PER_HOUR", 5),
		},
		Ops: OpsConfig{
			PprofEnabled: boolFromEnv("PPROF_ENABLED"),
		},
		TTL: TTLConfig{
			Options: map[string]time.Duration{
				"5m":   5 * time.Minute,
				"15m":  15 * time.Minute,
				"1h":   1 * time.Hour,
				"24h":  24 * time.Hour,
				"168h": 168 * time.Hour, // 7 days
			},
		},
	}
}

func listFromEnv(key, fallback string) []string {
	value := os.Getenv(key)
	if value == "" {
		value = fallback
	}
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(strings.TrimRight(part, "/")); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

// tokenTTLFromEnv parses TOKEN_TTL, defaulting to 24h. Malformed values fall
// back silently here and are rejected loudly by Validate at startup.
func tokenTTLFromEnv() time.Duration {
	if raw := os.Getenv("TOKEN_TTL"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			return parsed
		}
	}
	return 24 * time.Hour
}

// intFromEnv parses key as a non-negative integer, defaulting to fallback on
// empty or malformed values. A valid zero is accepted by the parser but
// rejected by Validate so an operator cannot accidentally disable
// registration.
func intFromEnv(key string, fallback int) int {
	if raw := os.Getenv(key); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			return parsed
		}
	}
	return fallback
}

// boolFromEnv parses key as an explicit boolean (true/1/yes/on). Everything
// else — including an empty value — stays false, so sensitive surfaces that
// are governed by this helper default to disabled unless explicitly enabled.
func boolFromEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Validate verifies configuration values that affect externally visible URLs.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	publicURL, err := url.Parse(c.Server.BaseURL)
	if err != nil || publicURL.Host == "" {
		return fmt.Errorf("PUBLIC_BASE_URL must be an absolute URL")
	}
	if publicURL.Scheme != "http" && publicURL.Scheme != "https" {
		return fmt.Errorf("PUBLIC_BASE_URL must use http or https")
	}
	if os.Getenv("GIN_MODE") == "release" && publicURL.Scheme != "https" && !isLoopbackHost(publicURL.Hostname()) {
		return fmt.Errorf("PUBLIC_BASE_URL must use https in release mode")
	}
	for _, origin := range c.Security.AllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" {
			return fmt.Errorf("ALLOWED_ORIGINS entries must be origins without paths")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("ALLOWED_ORIGINS entries must use http or https")
		}
	}
	if os.Getenv("GIN_MODE") == "release" && len([]byte(c.Security.MetricsToken)) < 32 {
		return fmt.Errorf("METRICS_TOKEN must be at least 32 bytes in release mode")
	}
	if raw := os.Getenv("TOKEN_TTL"); raw != "" {
		if _, err := time.ParseDuration(raw); err != nil {
			return fmt.Errorf("TOKEN_TTL must be a valid duration (e.g. 24h): %w", err)
		}
	}
	if err := validateLabel(c.Security.TokenIssuer, "TOKEN_ISSUER"); err != nil {
		return err
	}
	if err := validateLabel(c.Security.TokenAudience, "TOKEN_AUDIENCE"); err != nil {
		return err
	}
	if c.Security.RegisterLimitPerHour < 1 {
		return fmt.Errorf("REGISTER_LIMIT_PER_HOUR must be at least 1")
	}
	return nil
}

// validateLabel bounds optional token issuer/audience values and rejects
// whitespace or control characters.
func validateLabel(value, name string) error {
	if value == "" {
		return nil
	}
	if len(value) > 128 {
		return fmt.Errorf("%s must be at most 128 characters", name)
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7E {
			return fmt.Errorf("%s must not contain spaces or control characters", name)
		}
	}
	return nil
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
