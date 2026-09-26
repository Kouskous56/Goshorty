package services

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"goshorty/config"
	"goshorty/models"
	"goshorty/storage"
	"goshorty/utils"
)

// DefaultExpiresIn is the default TTL display string when none is specified
const DefaultExpiresIn = "24h"

// ErrForbidden is returned when a user tries to access a resource they don't own
var ErrForbidden = errors.New("access denied")

// ErrURLNotFound is returned when a URL does not exist or has expired.
var ErrURLNotFound = errors.New("URL not found or has expired")

// ErrCodeTaken is returned when a requested short code is already active.
var ErrCodeTaken = errors.New("short code is already taken")

// ErrInvalidURL is returned when the original URL is invalid.
var ErrInvalidURL = errors.New("invalid URL")

// URLService handles URL shortening logic
type URLService struct {
	storage storage.URLStore
	config  *config.Config
}

// NewURLService creates a new URL service
func NewURLService(st storage.URLStore, cfg *config.Config) *URLService {
	return &URLService{
		storage: st,
		config:  cfg,
	}
}

// CreateShortURL creates a new short URL
func (s *URLService) CreateShortURL(req *models.ShortenRequest, userID string) (*models.ShortenResponse, error) {
	// Validate request
	if req.URL == "" {
		return nil, fmt.Errorf("%w: URL is required", ErrInvalidURL)
	}

	u, err := url.Parse(req.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("%w: only http and https URLs are allowed", ErrInvalidURL)
	}

	// Determine TTL duration
	duration, ok := s.config.TTL.Options[req.ExpiresIn]
	if !ok {
		// Default to 24 hours if not specified or invalid
		duration = 24 * time.Hour
	}

	// Build the URL data before reserving a code. SetIfAbsent makes the
	// availability check and write atomic.
	now := time.Now()
	expiresAt := now.Add(duration)
	expiresInDisplay := req.ExpiresIn
	if expiresInDisplay == "" {
		expiresInDisplay = DefaultExpiresIn
	}

	urlData := &models.URLData{
		ID:          utils.GenerateID(),
		OriginalURL: req.URL,
		ExpiresIn:   expiresInDisplay,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		CreatedBy:   userID,
		Visits:      0,
	}

	var shortCode string
	if req.CustomCode != "" {
		shortCode = req.CustomCode
		urlData.ShortCode = shortCode
		if err := s.storage.SetIfAbsent(shortCode, urlData); err != nil {
			if errors.Is(err, storage.ErrKeyExists) {
				return nil, fmt.Errorf("%w: %s", ErrCodeTaken, shortCode)
			}
			return nil, fmt.Errorf("failed to store URL: %w", err)
		}
	} else {
		const maxAttempts = 100
		for i := 0; i < maxAttempts; i++ {
			codeLength := 6
			if i == maxAttempts-1 {
				codeLength = 10
			}
			shortCode = utils.GenerateShortCode(codeLength)
			urlData.ShortCode = shortCode
			err := s.storage.SetIfAbsent(shortCode, urlData)
			if err == nil {
				break
			}
			if !errors.Is(err, storage.ErrKeyExists) {
				return nil, fmt.Errorf("failed to store URL: %w", err)
			}
			shortCode = ""
		}
		if shortCode == "" {
			return nil, errors.New("failed to allocate a unique short code")
		}
	}

	response := &models.ShortenResponse{
		ID:          urlData.ID,
		ShortURL:    fmt.Sprintf("%s/r/%s", s.config.Server.BaseURL, shortCode),
		ShortCode:   shortCode,
		OriginalURL: req.URL,
		ExpiresIn:   expiresInDisplay,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}

	return response, nil
}

// GetOriginalURL retrieves the original URL and increments visit count atomically
func (s *URLService) GetOriginalURL(shortCode string) (string, error) {
	urlData, err := s.storage.GetAndIncrement(shortCode)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return "", ErrURLNotFound
		}
		return "", err
	}

	return urlData.OriginalURL, nil
}

// GetURLInfo retrieves information about a shortened URL
func (s *URLService) GetURLInfo(shortCode, userID, role string) (*models.URLData, error) {
	urlData, err := s.storage.Get(shortCode)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return nil, ErrURLNotFound
		}
		return nil, err
	}

	if role != models.RoleAdmin && urlData.CreatedBy != userID {
		return nil, ErrURLNotFound
	}

	return urlData, nil
}

// DeleteURL deletes a shortened URL (checks ownership unless admin)
func (s *URLService) DeleteURL(shortCode, userID, role string) error {
	urlData, err := s.storage.Get(shortCode)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return ErrURLNotFound
		}
		return fmt.Errorf("failed to read URL: %w", err)
	}

	if role != models.RoleAdmin && urlData.CreatedBy != userID {
		return ErrForbidden
	}

	if err := s.storage.Delete(shortCode); err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return ErrURLNotFound
		}
		return fmt.Errorf("failed to delete URL: %w", err)
	}
	return nil
}

// GetAllURLs returns URLs visible to the caller, newest first.
func (s *URLService) GetAllURLs(userID, role string) ([]*models.URLData, error) {
	urls, err := s.storage.GetAllFor(userID, role)
	if err != nil {
		return nil, fmt.Errorf("failed to list URLs: %w", err)
	}
	sortURLs(urls)
	return urls, nil
}

// ListURLs returns a single page of URLs visible to the caller, newest first,
// with a keyset cursor for the next page. The cursor token is opaque; the
// page slice itself is computed by the storage backend so the whole table is
// never loaded into memory on the Postgres path.
func (s *URLService) ListURLs(userID, role string, opts ListOptions) (URLPage, error) {
	limit := NormalizeLimit(opts.Limit)

	var cursor models.URLCursor
	if opts.Cursor != "" {
		if err := models.DecodeCursor(opts.Cursor, &cursor); err != nil {
			return URLPage{}, ErrInvalidCursor
		}
		if cursor.CreatedAtUnixNano == 0 || cursor.ShortCode == "" {
			return URLPage{}, ErrInvalidCursor
		}
	}

	res, err := s.storage.ListURLs(userID, role, cursor, limit)
	if err != nil {
		return URLPage{}, fmt.Errorf("failed to list URLs: %w", err)
	}

	page := URLPage{
		URLs:    res.Items,
		Total:   res.Total,
		HasMore: res.HasMore,
	}
	if res.HasMore && len(res.Items) > 0 {
		last := res.Items[len(res.Items)-1]
		enc, err := models.EncodeCursor(models.URLCursor{
			CreatedAtUnixNano: last.CreatedAt.UnixNano(),
			ShortCode:         last.ShortCode,
		})
		if err != nil {
			return URLPage{}, fmt.Errorf("encode next cursor: %w", err)
		}
		page.NextCursor = enc
	}
	return page, nil
}

// GetStats returns statistics
func (s *URLService) GetStats(userID, role string) (map[string]interface{}, error) {
	stats, err := s.storage.StatsFor(userID, role)
	if err != nil {
		return nil, fmt.Errorf("failed to load statistics: %w", err)
	}
	return stats, nil
}
