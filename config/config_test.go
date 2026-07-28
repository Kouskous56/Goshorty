package config

import "testing"

func TestPublicBaseURLPriorityAndNormalization(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "https://short.example.com/")
	t.Setenv("BASE_URL", "https://legacy.example.com")

	cfg := NewConfig()
	if cfg.Server.BaseURL != "https://short.example.com" {
		t.Fatalf("unexpected base URL: %s", cfg.Server.BaseURL)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestConfigRejectsInvalidPublicBaseURL(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "javascript:alert(1)")

	if err := NewConfig().Validate(); err == nil {
		t.Fatal("expected invalid public base URL to be rejected")
	}
}

func TestDatabaseURLFromEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/goshorty")

	cfg := NewConfig()
	if cfg.Database.URL != "postgres://user:pass@localhost:5432/goshorty" {
		t.Fatalf("unexpected database URL: %s", cfg.Database.URL)
	}
}
