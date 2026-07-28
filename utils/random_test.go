package utils

import (
	"regexp"
	"testing"
)

func TestGenerateShortCode(t *testing.T) {
	code := GenerateShortCode(32)
	if len(code) != 32 {
		t.Fatalf("expected length 32, got %d", len(code))
	}
	if !regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(code) {
		t.Fatalf("code contains non URL-safe characters: %q", code)
	}
}
