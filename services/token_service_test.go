package services

import (
	"strings"
	"testing"
	"time"
)

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
	tampered := parts[0] + "." + parts[1][:len(parts[1])-1] + "x"
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
	previousTTL := TokenTTL
	TokenTTL = -time.Second
	t.Cleanup(func() { TokenTTL = previousTTL })

	service := NewTokenService("test-secret")
	token, err := service.GenerateToken("user-id", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.VerifyToken(token); err == nil {
		t.Fatal("expected expired token to fail")
	}
}
