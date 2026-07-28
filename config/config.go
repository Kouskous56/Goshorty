package config

import (
	"os"
	"time"
)

// Config holds application configuration
type Config struct {
	Server ServerConfig
	TTL    TTLConfig
}

// ServerConfig holds server settings
type ServerConfig struct {
	Port    string
	BaseURL string // e.g., "http://localhost:8080"
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
	if u := os.Getenv("BASE_URL"); u != "" {
		return u
	}
	if p := os.Getenv("PORT"); p != "" {
		return "http://localhost:" + p
	}
	return defaultURL
}

// NewConfig creates a new configuration with default values
func NewConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:    portFromEnv(":8080"),
			BaseURL: baseURLFromEnv("http://localhost:8080"),
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
