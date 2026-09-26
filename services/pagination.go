package services

import (
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

// sortURLs orders newest first (created_at DESC) with short_code ASC as the
// deterministic tie-breaker. The comparator lives on models.URLData so the
// storage backends reuse the exact same ordering rule.
func sortURLs(all []*models.URLData) {
	sort.SliceStable(all, func(i, j int) bool { return all[i].NewerThan(all[j]) })
}
