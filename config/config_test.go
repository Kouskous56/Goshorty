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

func TestSecurityConfig(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "https://short.example.com")
	t.Setenv("ALLOWED_ORIGINS", "https://short.example.com, http://localhost:3000/")
	t.Setenv("TRUSTED_PROXIES", "100.64.0.0/10,127.0.0.1")

	cfg := NewConfig()
	if len(cfg.Security.AllowedOrigins) != 2 || cfg.Security.AllowedOrigins[1] != "http://localhost:3000" {
		t.Fatalf("unexpected allowed origins: %#v", cfg.Security.AllowedOrigins)
	}
	if len(cfg.Security.TrustedProxies) != 2 {
		t.Fatalf("unexpected trusted proxies: %#v", cfg.Security.TrustedProxies)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsAllowedOriginWithPath(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "https://short.example.com")
	t.Setenv("ALLOWED_ORIGINS", "https://short.example.com/path")
	if err := NewConfig().Validate(); err == nil {
		t.Fatal("expected origin containing a path to be rejected")
	}
}

func TestReleaseRequiresStrongMetricsToken(t *testing.T) {
	t.Setenv("GIN_MODE", "release")
	t.Setenv("PUBLIC_BASE_URL", "https://short.example.com")
	t.Setenv("METRICS_TOKEN", "short")
	if err := NewConfig().Validate(); err == nil {
		t.Fatal("expected weak release metrics token to be rejected")
	}

	t.Setenv("METRICS_TOKEN", "metrics-token-with-at-least-32-bytes")
	if err := NewConfig().Validate(); err != nil {
		t.Fatalf("expected strong metrics token: %v", err)
	}
}

func TestReleaseRequiresHTTPSExceptLoopback(t *testing.T) {
	t.Setenv("GIN_MODE", "release")
	t.Setenv("METRICS_TOKEN", "metrics-token-with-at-least-32-bytes")
	t.Setenv("PUBLIC_BASE_URL", "http://short.example.com")
	if err := NewConfig().Validate(); err == nil {
		t.Fatal("expected public release URL without HTTPS to be rejected")
	}

	t.Setenv("PUBLIC_BASE_URL", "http://127.0.0.1:8080")
	t.Setenv("ALLOWED_ORIGINS", "http://127.0.0.1:8080")
	if err := NewConfig().Validate(); err != nil {
		t.Fatalf("loopback release URL should remain available for CI: %v", err)
	}
}

func TestRegisterLimitPerHourFromEnvironment(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "https://short.example.com")
	t.Setenv("REGISTER_LIMIT_PER_HOUR", "50")

	cfg := NewConfig()
	if cfg.Security.RegisterLimitPerHour != 50 {
		t.Fatalf("unexpected register limit: %d", cfg.Security.RegisterLimitPerHour)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestRegisterLimitPerHourDefaultFallbackAndRejectsZero(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "https://short.example.com")

	if got := NewConfig().Security.RegisterLimitPerHour; got != 5 {
		t.Fatalf("default register limit = %d, want 5", got)
	}

	t.Setenv("REGISTER_LIMIT_PER_HOUR", "0")
	if err := NewConfig().Validate(); err == nil {
		t.Fatal("expected a register limit of 0 to be rejected")
	}

	t.Setenv("REGISTER_LIMIT_PER_HOUR", "not-a-number")
	if got := NewConfig().Security.RegisterLimitPerHour; got != 5 {
		t.Fatalf("malformed register limit = %d, want fallback 5", got)
	}
}

func TestPprofDisabledByDefault(t *testing.T) {
	t.Setenv("PPROF_ENABLED", "")
	if NewConfig().Ops.PprofEnabled {
		t.Fatal("pprof must be disabled by default")
	}
}

func TestPprofEnabledFromEnvironment(t *testing.T) {
	for _, value := range []string{"true", "1", "yes", "on"} {
		t.Setenv("PPROF_ENABLED", value)
		if !NewConfig().Ops.PprofEnabled {
			t.Fatalf("PPROF_ENABLED=%s should enable pprof", value)
		}
	}
}

func TestPprofIgnoresNonExplicitValues(t *testing.T) {
	for _, value := range []string{"false", "0", "no", "garbage", " TRUE "} {
		t.Setenv("PPROF_ENABLED", value)
		if NewConfig().Ops.PprofEnabled {
			t.Fatalf("PPROF_ENABLED=%q must leave pprof disabled", value)
		}
	}
}
