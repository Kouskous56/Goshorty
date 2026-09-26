package services

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type fakeClock struct {
	t time.Time
}

func (c *fakeClock) now() time.Time { return c.t }
func (c *fakeClock) advance(d time.Duration) {
	c.t = c.t.Add(d)
}

func TestTokenRoundTripAndTamperResistance(t *testing.T) {
	service := NewTokenService("test-secret")
	token, err := service.GenerateToken("user-id", "alice", "user", 7)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.VerifyToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "user-id" || claims.Username != "alice" || claims.Role != "user" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.TokenVersion != 7 {
		t.Fatalf("expected token version 7, got %d", claims.TokenVersion)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatal("generated token should be payload.signature")
	}
	// Mutate a payload character in the middle of the base64 string, where all
	// six bits are significant. Mutating only the final character of the
	// signature is a broken tamper: the trailing two bits of that character are
	// padding that the decoder drops, so swapping w/x/y/z (which share the same
	// four payload bits) can leave the decoded signature byte-for-byte
	// identical and the "tampered" token still verifies.
	payload := parts[0]
	pos := len(payload) / 2
	mutated := payload[:pos] + "A" + payload[pos+1:]
	if mutated == payload {
		mutated = payload[:pos] + "B" + payload[pos+1:]
	}
	tampered := mutated + "." + parts[1]
	if _, err := service.VerifyToken(tampered); err == nil {
		t.Fatal("expected tampered token to fail")
	}
	for _, invalid := range []string{"", "one-part", "!!!.signature"} {
		if _, err := service.VerifyToken(invalid); err == nil {
			t.Fatalf("expected invalid token %q to fail", invalid)
		}
	}
}

func TestExpiredTokenIsRejected(t *testing.T) {
	service := NewTokenService("test-secret", WithTTL(-time.Second))
	token, err := service.GenerateToken("user-id", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.VerifyToken(token); err == nil {
		t.Fatal("expected invalid (zero-lifetime) token to fail")
	}
}

func TestTokenExpiresPerConfiguredTTL(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)}
	service := NewTokenService("test-secret", WithTTL(time.Hour), WithClock(clock.now))
	token, err := service.GenerateToken("user-id", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}

	clock.advance(59 * time.Minute)
	if _, err := service.VerifyToken(token); err != nil {
		t.Fatalf("expected token valid within TTL: %v", err)
	}

	clock.advance(2 * time.Minute)
	if _, err := service.VerifyToken(token); err == nil {
		t.Fatal("expected token to expire after configured TTL")
	}
}

func TestTokensCarryUniqueJTI(t *testing.T) {
	service := NewTokenService("test-secret")
	first, err := service.GenerateToken("user-id", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.GenerateToken("user-id", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}
	firstClaims, err := service.VerifyToken(first)
	if err != nil {
		t.Fatal(err)
	}
	secondClaims, err := service.VerifyToken(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstClaims.JTI == "" {
		t.Fatal("expected a non-empty jti claim")
	}
	if firstClaims.JTI == secondClaims.JTI {
		t.Fatal("expected different jti claims per token")
	}
}

func TestKeyRotationAcceptsLegacyTokens(t *testing.T) {
	legacyService := NewTokenService("old-secret")
	legacy, err := legacyService.GenerateToken("user-id", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}

	rotated := NewTokenService("new-secret", WithPreviousSecret("old-secret"))
	if _, err := rotated.VerifyToken(legacy); err != nil {
		t.Fatalf("legacy token should verify during rotation: %v", err)
	}

	fresh, err := rotated.GenerateToken("user-id", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotated.VerifyToken(fresh); err != nil {
		t.Fatalf("fresh token should verify: %v", err)
	}

	// After rotation completes (previous key removed), legacy tokens stop
	// working but tokens minted with the active key remain valid.
	completed := NewTokenService("new-secret")
	if _, err := completed.VerifyToken(legacy); err == nil {
		t.Fatal("expected legacy token to fail after previous key removed")
	}
	if _, err := completed.VerifyToken(fresh); err != nil {
		t.Fatalf("expected fresh token to remain valid: %v", err)
	}

	// A token signed with an unrelated key must never verify.
	alien := NewTokenService("alien-secret")
	evil, err := alien.GenerateToken("user-id", "mallory", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotated.VerifyToken(evil); err == nil {
		t.Fatal("expected alien token to be rejected")
	}
}

func TestTokenIssuerAndAudience(t *testing.T) {
	service := NewTokenService("test-secret",
		WithIssuer("https://goshorty.example"),
		WithAudience("goshorty-api"),
	)
	token, err := service.GenerateToken("user-id", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.VerifyToken(token)
	if err != nil {
		t.Fatalf("expected matching issuer/audience token to verify: %v", err)
	}
	if claims.Iss != "https://goshorty.example" || claims.Aud != "goshorty-api" {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	// A verifier with a different issuer rejects the same token.
	mismatch := NewTokenService("test-secret",
		WithIssuer("https://other.example"),
		WithAudience("goshorty-api"),
	)
	if _, err := mismatch.VerifyToken(token); err == nil {
		t.Fatal("expected mismatched issuer to be rejected")
	}

	// Without issuer/audience configured (backward-compatible mode) the token
	// is accepted.
	relaxed := NewTokenService("test-secret")
	if _, err := relaxed.VerifyToken(token); err != nil {
		t.Fatalf("expected relaxed mode to accept token: %v", err)
	}

	// Enabling enforcement invalidates pre-existing tokens lacking claims.
	legacy, err := NewTokenService("old-secret").GenerateToken("user-id", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}
	enforced := NewTokenService("old-secret",
		WithIssuer("https://goshorty.example"),
		WithAudience("goshorty-api"),
	)
	if _, err := enforced.VerifyToken(legacy); err == nil {
		t.Fatal("expected legacy token without issuer/audience to be rejected when enforced")
	}
}

func TestFutureIssuedAtRejected(t *testing.T) {
	base := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{t: base}
	service := NewTokenService("test-secret", WithTTL(time.Hour), WithClock(clock.now))
	token, err := service.GenerateToken("user-id", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}

	// Verifying on a clock well before issuance must fail.
	clock.t = base.Add(-10 * time.Minute)
	if _, err := service.VerifyToken(token); err == nil {
		t.Fatal("expected token issued in the future to be rejected")
	}

	// Small clock skew is tolerated.
	clock.t = base.Add(-10 * time.Second)
	if _, err := service.VerifyToken(token); err != nil {
		t.Fatalf("expected token within clock skew to verify: %v", err)
	}
}

func TestTokenLifetimeCannotExceedConfiguredTTL(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)}
	service := NewTokenService("test-secret", WithTTL(time.Hour), WithClock(clock.now))

	// Craft a token claiming a two-hour lifetime.
	claims := TokenClaims{
		UserID:    "user-id",
		Username:  "alice",
		Role:      "user",
		IssuedAt:  clock.t.Add(-2 * time.Hour).Unix(),
		ExpiresAt: clock.t.Unix(),
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	sig := base64.RawURLEncoding.EncodeToString(signBytes(payload, []byte("test-secret")))
	token := payload + "." + sig

	if _, err := service.VerifyToken(token); err == nil {
		t.Fatal("expected token with lifetime above configured TTL to be rejected")
	}
}

func TestOversizedTokenRejected(t *testing.T) {
	service := NewTokenService("test-secret")
	payload := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("A", maxTokenLength)))
	token := payload + "." + base64.RawURLEncoding.EncodeToString([]byte("AAAA"))
	if _, err := service.VerifyToken(token); err == nil {
		t.Fatal("expected oversized token to be rejected")
	}
}

func TestSignatureMustBeSHA256Size(t *testing.T) {
	service := NewTokenService("test-secret")
	token, err := service.GenerateToken("user-id", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")

	shortSig := base64.RawURLEncoding.EncodeToString([]byte("short"))
	if _, err := service.VerifyToken(parts[0] + "." + shortSig); err == nil {
		t.Fatal("expected short signature to be rejected")
	}

	longSig := base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	if _, err := service.VerifyToken(parts[0] + "." + longSig); err == nil {
		t.Fatal("expected oversized signature to be rejected")
	}
}

func TestUnknownKeyIDRejected(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)}
	service := NewTokenService("test-secret", WithTTL(time.Hour), WithClock(clock.now))

	claims := TokenClaims{
		JTI:       "abc",
		UserID:    "user-id",
		Username:  "alice",
		Role:      "user",
		KID:       "v999",
		IssuedAt:  clock.t.Add(-time.Minute).Unix(),
		ExpiresAt: clock.t.Add(59 * time.Minute).Unix(),
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	sig := base64.RawURLEncoding.EncodeToString(signBytes(payload, []byte("test-secret")))
	token := payload + "." + sig

	if _, err := service.VerifyToken(token); err == nil {
		t.Fatal("expected unknown kid to be rejected")
	}
}
