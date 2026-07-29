package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TokenClaims represents JWT-like claims
type TokenClaims struct {
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	TokenVersion int64  `json:"token_version"`
	IssuedAt     int64  `json:"issued_at"`
	ExpiresAt    int64  `json:"expires_at"`
}

// TokenTTL is the duration before a token expires (configurable via TOKEN_TTL env var)
var TokenTTL = 24 * time.Hour

// TokenService handles token creation and validation with HMAC-SHA256 signing
type TokenService struct {
	secretKey []byte
}

// NewTokenService creates a new token service with the given secret key.
// If secret is empty, the service will reject all tokens.
func NewTokenService(secret string) *TokenService {
	return &TokenService{
		secretKey: []byte(secret),
	}
}

// GenerateToken creates a signed token: base64(payload).base64(signature)
func (ts *TokenService) GenerateToken(userID, username, role string, tokenVersion ...int64) (string, error) {
	version := int64(0)
	if len(tokenVersion) > 0 {
		version = tokenVersion[0]
	}
	claims := TokenClaims{
		UserID:       userID,
		Username:     username,
		Role:         role,
		TokenVersion: version,
		IssuedAt:     time.Now().Unix(),
		ExpiresAt:    time.Now().Add(TokenTTL).Unix(),
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}

	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	sig := ts.sign(payload)
	token := payload + "." + sig

	return token, nil
}

// VerifyToken verifies the HMAC signature and returns claims
func (ts *TokenService) VerifyToken(token string) (*TokenClaims, error) {
	if token == "" {
		return nil, errors.New("token is empty")
	}

	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid token format")
	}

	payload := parts[0]
	sig := parts[1]

	// Verify signature
	expectedSig := ts.sign(payload)
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return nil, errors.New("invalid token signature")
	}

	// Decode payload
	claimsJSON, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, errors.New("invalid token encoding")
	}

	var claims TokenClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, errors.New("invalid token payload")
	}

	// Check expiration
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, errors.New("token expired")
	}

	return &claims, nil
}

// sign creates an HMAC-SHA256 signature of the payload
func (ts *TokenService) sign(payload string) string {
	mac := hmac.New(sha256.New, ts.secretKey)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
