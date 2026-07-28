package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config holds application configuration
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
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

// TTLConfig holds TTL duration settings
type TTLConfig struct {
	Options map[string]time.Duration
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
	return &Config{
		Server: ServerConfig{
			Port:    portFromEnv(":8080"),
			BaseURL: baseURLFromEnv("http://localhost:8080"),
		},
		Database: DatabaseConfig{
			URL: os.Getenv("DATABASE_URL"),
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
	return nil
}
