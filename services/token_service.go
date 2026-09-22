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

	"goshorty/utils"
)

const (
	// defaultTokenTTL is used when no TTL option is supplied.
	defaultTokenTTL = 24 * time.Hour
	// maxTokenLength bounds the wire size of a token so that malformed or
	// oversized inputs cannot trigger unbounded parsing work.
	maxTokenLength = 4096
	// clockSkew is tolerated when validating a token's issued_at timestamp.
	clockSkew = 30 * time.Second
	// activeKeyID is stamped on every new token. A previous key is only
	// accepted for legacy tokens that carry no kid claim, which enables smooth
	// key rotation without a global re-login.
	activeKeyID = "v1"
)

// TokenClaims represents the signed payload carried by a token.
type TokenClaims struct {
	JTI          string `json:"jti"`
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	TokenVersion int64  `json:"token_version"`
	Iss          string `json:"iss"`
	Aud          string `json:"aud"`
	KID          string `json:"kid"`
	IssuedAt     int64  `json:"issued_at"`
	ExpiresAt    int64  `json:"expires_at"`
}

// Option configures a TokenService at construction time.
type Option func(*TokenService)

// WithTTL overrides the default 24h token lifetime. Generated tokens expire
// ttl after issuance, and verification rejects any token whose lifetime
// exceeds this value.
func WithTTL(ttl time.Duration) Option {
	return func(ts *TokenService) { ts.ttl = ttl }
}

// WithPreviousSecret accepts legacy kid-less tokens signed with a previously
// active HMAC key, enabling smooth key rotation. Once the previous secret is
// removed, those tokens stop verifying.
func WithPreviousSecret(secret string) Option {
	return func(ts *TokenService) {
		if secret != "" {
			ts.previousKey = []byte(secret)
		}
	}
}

// WithIssuer stamps an issuer claim on new tokens and rejects tokens whose
// issuer does not match on verification. Empty values disable the check.
func WithIssuer(issuer string) Option {
	return func(ts *TokenService) { ts.issuer = issuer }
}

// WithAudience stamps an audience claim on new tokens and rejects tokens whose
// audience does not match on verification. Empty values disable the check.
func WithAudience(audience string) Option {
	return func(ts *TokenService) { ts.audience = audience }
}

// WithClock overrides the time source, primarily for deterministic tests.
func WithClock(now func() time.Time) Option {
	return func(ts *TokenService) { ts.now = now }
}

// TokenService signs and verifies HMAC-SHA256 tokens of the form
// base64url(payload).base64url(signature).
type TokenService struct {
	secretKey   []byte
	previousKey []byte
	ttl         time.Duration
	issuer      string
	audience    string
	now         func() time.Time
}

// NewTokenService creates a token service with the given secret key. If secret
// is empty, the service rejects all tokens.
func NewTokenService(secret string, options ...Option) *TokenService {
	ts := &TokenService{
		secretKey: []byte(secret),
		ttl:       defaultTokenTTL,
		now:       time.Now,
	}
	for _, opt := range options {
		opt(ts)
	}
	return ts
}

// GenerateToken creates a signed token: base64(payload).base64(signature)
func (ts *TokenService) GenerateToken(userID, username, role string, tokenVersion ...int64) (string, error) {
	if len(ts.secretKey) == 0 {
		return "", errors.New("token service has no signing key")
	}

	version := int64(0)
	if len(tokenVersion) > 0 {
		version = tokenVersion[0]
	}

	now := ts.now().Unix()
	claims := TokenClaims{
		JTI:          utils.GenerateID(),
		UserID:       userID,
		Username:     username,
		Role:         role,
		TokenVersion: version,
		Iss:          ts.issuer,
		Aud:          ts.audience,
		KID:          activeKeyID,
		IssuedAt:     now,
		ExpiresAt:    now + int64(ts.ttl.Seconds()),
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}

	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	sig := base64.RawURLEncoding.EncodeToString(signBytes(payload, ts.secretKey))
	return payload + "." + sig, nil
}

// VerifyToken verifies the HMAC signature and returns claims.
//
// The signature is checked before the payload is parsed, and a signature is
// accepted from the active key or, for legacy kid-less tokens, a configured
// previous key during rotation.
func (ts *TokenService) VerifyToken(token string) (*TokenClaims, error) {
	if token == "" {
		return nil, errors.New("token is empty")
	}
	if len(token) > maxTokenLength {
		return nil, errors.New("token is too large")
	}
	if len(ts.secretKey) == 0 {
		return nil, errors.New("token service has no signing key")
	}

	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid token format")
	}
	payload, sigB64 := parts[0], parts[1]

	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil || len(sig) != sha256.Size {
		return nil, errors.New("invalid token signature")
	}

	valid := hmac.Equal(sig, signBytes(payload, ts.secretKey))
	if !valid && len(ts.previousKey) > 0 {
		valid = hmac.Equal(sig, signBytes(payload, ts.previousKey))
	}
	if !valid {
		return nil, errors.New("invalid token signature")
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, errors.New("invalid token encoding")
	}

	var claims TokenClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, errors.New("invalid token payload")
	}

	now := ts.now().Unix()
	if claims.IssuedAt <= 0 || claims.ExpiresAt <= claims.IssuedAt {
		return nil, errors.New("invalid token timestamps")
	}
	if claims.IssuedAt > now+int64(clockSkew.Seconds()) {
		return nil, errors.New("token issued in the future")
	}
	if now > claims.ExpiresAt {
		return nil, errors.New("token expired")
	}
	if claims.ExpiresAt-claims.IssuedAt > int64(ts.ttl.Seconds()) {
		return nil, errors.New("token lifetime exceeds configured TTL")
	}
	if claims.KID != "" && claims.KID != activeKeyID {
		return nil, errors.New("unknown token key id")
	}
	if ts.issuer != "" && claims.Iss != ts.issuer {
		return nil, errors.New("invalid token issuer")
	}
	if ts.audience != "" && claims.Aud != ts.audience {
		return nil, errors.New("invalid token audience")
	}

	return &claims, nil
}

// signBytes creates the raw HMAC-SHA256 digest of the payload.
func signBytes(payload string, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
