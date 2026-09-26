package models

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

// ErrMalformedCursor is returned when a pagination cursor cannot be decoded.
// Callers map it to their own domain error (e.g. services.ErrInvalidCursor).
var ErrMalformedCursor = errors.New("malformed pagination cursor")

// URLCursor is the keyset position for URL list pagination. The key is
// (created_at DESC, short_code ASC) — creation wall time in nanoseconds plus
// the short code as a deterministic tie-breaker.
type URLCursor struct {
	CreatedAtUnixNano int64  `json:"c"`
	ShortCode         string `json:"s"`
}

// UserCursor is the keyset position for user list pagination (username ASC).
type UserCursor struct {
	Username string `json:"u"`
}

// EncodeCursor serializes a cursor struct into an opaque, URL-safe token.
func EncodeCursor(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// DecodeCursor parses an opaque cursor token into out. It returns
// ErrMalformedCursor for malformed input.
func DecodeCursor(raw string, out any) error {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return ErrMalformedCursor
	}
	if err := json.Unmarshal(b, out); err != nil {
		return ErrMalformedCursor
	}
	return nil
}

// NewerThan implements the URL list ordering comparator
// (created_at DESC, short_code ASC): newer creation time first, with the
// short code as a deterministic tie-breaker.
func (u *URLData) NewerThan(other *URLData) bool {
	n1, n2 := u.CreatedAt.UnixNano(), other.CreatedAt.UnixNano()
	if n1 != n2 {
		return n1 > n2
	}
	return u.ShortCode < other.ShortCode
}

// AfterKeyset reports whether u sorts after the keyset position cur — i.e. it
// belongs on a later page of the (created_at DESC, short_code ASC) ordering.
func (u *URLData) AfterKeyset(cur URLCursor) bool {
	n := u.CreatedAt.UnixNano()
	if n != cur.CreatedAtUnixNano {
		return n < cur.CreatedAtUnixNano
	}
	return u.ShortCode > cur.ShortCode
}
