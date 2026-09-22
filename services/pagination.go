package services

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"

	"goshorty/models"
)

// ListOptions carries pagination parameters for list endpoints.
// Limit 0 means the default; a negative or oversized value is normalized.
type ListOptions struct {
	// Limit is the maximum number of items per page (1..MaxPageLimit).
	Limit int
	// Cursor is an opaque keyset cursor returned as next_cursor on a
	// previous page. Empty means "first page".
	Cursor string
}

const (
	// DefaultPageLimit is used when no limit is supplied.
	DefaultPageLimit = 50
	// MaxPageLimit caps a requested page size.
	MaxPageLimit = 200
)

// ErrInvalidCursor is returned when a cursor cannot be decoded.
var ErrInvalidCursor = errors.New("invalid pagination cursor")

// NormalizeLimit returns the limit clamped to [1, MaxPageLimit], defaulting
// to DefaultPageLimit for non-positive values.
func NormalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultPageLimit
	}
	if limit > MaxPageLimit {
		return MaxPageLimit
	}
	return limit
}

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

// URLPage is one page of a URL listing.
type URLPage struct {
	URLs       []*models.URLData
	NextCursor string // empty on the last page
	Total      int
	HasMore    bool
}

// UserPage is one page of a user listing.
type UserPage struct {
	Users      []*models.User
	NextCursor string // empty on the last page
	Total      int
	HasMore    bool
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
// ErrInvalidCursor for malformed input.
func DecodeCursor(raw string, out any) error {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return ErrInvalidCursor
	}
	if err := json.Unmarshal(b, out); err != nil {
		return ErrInvalidCursor
	}
	return nil
}

// sortURLs orders newest first (created_at DESC) with short_code ASC as the
// deterministic tie-breaker.
func sortURLs(all []*models.URLData) {
	sort.SliceStable(all, func(i, j int) bool {
		ni := all[i].CreatedAt.UnixNano()
		nj := all[j].CreatedAt.UnixNano()
		if ni != nj {
			return ni > nj
		}
		return all[i].ShortCode < all[j].ShortCode
	})
}

// urlKeyAfter reports whether e sorts after the keyset position cur in the
// (created_at DESC, short_code ASC) ordering — i.e. e belongs on a later page.
func urlKeyAfter(e *models.URLData, cur URLCursor) bool {
	n := e.CreatedAt.UnixNano()
	if n != cur.CreatedAtUnixNano {
		return n < cur.CreatedAtUnixNano
	}
	return e.ShortCode > cur.ShortCode
}

// PaginateURLs sorts all (newest first) and returns the requested page.
// The returned cursor is safe to pass back for the next page even if items
// expired or were deleted between calls (keyset semantics).
func PaginateURLs(all []*models.URLData, opts ListOptions) (URLPage, error) {
	sortURLs(all)
	total := len(all)
	limit := NormalizeLimit(opts.Limit)

	start := 0
	if opts.Cursor != "" {
		var cur URLCursor
		if err := DecodeCursor(opts.Cursor, &cur); err != nil {
			return URLPage{}, ErrInvalidCursor
		}
		if cur.CreatedAtUnixNano == 0 || cur.ShortCode == "" {
			return URLPage{}, ErrInvalidCursor
		}
		start = sort.Search(total, func(i int) bool { return urlKeyAfter(all[i], cur) })
	}

	if start >= total {
		return URLPage{URLs: []*models.URLData{}, Total: total}, nil
	}
	end := start + limit
	if end > total {
		end = total
	}
	hasMore := end < total
	page := all[start:end]

	next := ""
	if hasMore {
		last := page[len(page)-1]
		enc, err := EncodeCursor(URLCursor{
			CreatedAtUnixNano: last.CreatedAt.UnixNano(),
			ShortCode:         last.ShortCode,
		})
		if err != nil {
			return URLPage{}, err
		}
		next = enc
	}
	return URLPage{URLs: page, NextCursor: next, Total: total, HasMore: hasMore}, nil
}

// PaginateUsers sorts all by username (ASC) and returns the requested page.
func PaginateUsers(all []*models.User, opts ListOptions) (UserPage, error) {
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].Username < all[j].Username
	})
	total := len(all)
	limit := NormalizeLimit(opts.Limit)

	start := 0
	if opts.Cursor != "" {
		var cur UserCursor
		if err := DecodeCursor(opts.Cursor, &cur); err != nil {
			return UserPage{}, ErrInvalidCursor
		}
		if cur.Username == "" {
			return UserPage{}, ErrInvalidCursor
		}
		start = sort.Search(total, func(i int) bool { return all[i].Username > cur.Username })
	}

	if start >= total {
		return UserPage{Users: []*models.User{}, Total: total}, nil
	}
	end := start + limit
	if end > total {
		end = total
	}
	hasMore := end < total
	page := all[start:end]

	next := ""
	if hasMore {
		last := page[len(page)-1]
		enc, err := EncodeCursor(UserCursor{Username: last.Username})
		if err != nil {
			return UserPage{}, err
		}
		next = enc
	}
	return UserPage{Users: page, NextCursor: next, Total: total, HasMore: hasMore}, nil
}
