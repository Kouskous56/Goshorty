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

// URLService handles URL shortening logic
type URLService struct {
	storage *storage.Storage
	config  *config.Config
}

// NewURLService creates a new URL service
func NewURLService(st *storage.Storage, cfg *config.Config) *URLService {
	return &URLService{
		storage: st,
		config:  cfg,
	}
}

// CreateShortURL creates a new short URL
func (s *URLService) CreateShortURL(req *models.ShortenRequest, userID string) (*models.ShortenResponse, error) {
	// Validate request
	if req.URL == "" {
		return nil, errors.New("URL is required")
	}

	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("only http and https URLs are allowed")
	}

	// Determine TTL duration
	duration, ok := s.config.TTL.Options[req.ExpiresIn]
	if !ok {
		// Default to 24 hours if not specified or invalid
		duration = 24 * time.Hour
	}

	// Generate short code
	var shortCode string
	if req.CustomCode != "" {
		// Check if custom code is available
		if s.storage.Exists(req.CustomCode) {
			return nil, fmt.Errorf("custom code '%s' is already taken", req.CustomCode)
		}
		shortCode = req.CustomCode
	} else {
		// Generate random code and ensure uniqueness
		shortCode = s.generateUniqueCode()
	}

	// Create URL data
	now := time.Now()
	expiresAt := now.Add(duration)
	
	urlData := &models.URLData{
		ID:          utils.GenerateID(),
		ShortCode:   shortCode,
		OriginalURL: req.URL,
		ExpiresIn:   req.ExpiresIn,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		CreatedBy:   userID,
		Visits:      0,
	}

	// Store in storage
	if err := s.storage.Set(shortCode, urlData); err != nil {
		return nil, fmt.Errorf("failed to store URL: %w", err)
	}

	// Build response
	expiresInDisplay := req.ExpiresIn
	if expiresInDisplay == "" {
		expiresInDisplay = DefaultExpiresIn
	}

	response := &models.ShortenResponse{
		ID:           urlData.ID,
		ShortURL:     fmt.Sprintf("%s/goshorty/%s/%s", s.config.Server.BaseURL, expiresInDisplay, shortCode),
		ShortCode:    shortCode,
		OriginalURL:  req.URL,
		ExpiresIn:    expiresInDisplay,
		ExpiresAt:    expiresAt,
		CreatedAt:    now,
	}

	return response, nil
}

// GetOriginalURL retrieves the original URL and increments visit count atomically
func (s *URLService) GetOriginalURL(shortCode string) (string, error) {
	urlData, err := s.storage.GetAndIncrement(shortCode)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return "", errors.New("URL not found or has expired")
		}
		return "", err
	}

	return urlData.OriginalURL, nil
}

// GetURLInfo retrieves information about a shortened URL
func (s *URLService) GetURLInfo(shortCode string) (*models.URLData, error) {
	urlData, err := s.storage.Get(shortCode)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return nil, errors.New("URL not found or has expired")
		}
		return nil, err
	}

	return urlData, nil
}

// DeleteURL deletes a shortened URL (checks ownership unless admin)
func (s *URLService) DeleteURL(shortCode, userID, role string) error {
	urlData, err := s.storage.Get(shortCode)
	if err != nil {
		return err
	}

	if role != models.RoleAdmin && urlData.CreatedBy != userID {
		return ErrForbidden
	}

	return s.storage.Delete(shortCode)
}

// GetAllURLs returns URLs visible to the caller
func (s *URLService) GetAllURLs(userID, role string) []*models.URLData {
	all := s.storage.GetAll()
	if role == models.RoleAdmin {
		return all
	}

	filtered := make([]*models.URLData, 0, len(all))
	for _, u := range all {
		if u.CreatedBy == userID {
			filtered = append(filtered, u)
		}
	}
	return filtered
}

// GetStats returns statistics
func (s *URLService) GetStats() map[string]interface{} {
	return s.storage.Stats()
}

// generateUniqueCode generates a unique short code
func (s *URLService) generateUniqueCode() string {
	const maxAttempts = 100
	
	for i := 0; i < maxAttempts; i++ {
		code := utils.GenerateShortCode(6)
		if !s.storage.Exists(code) {
			return code
		}
	}

	// If we reach here, generate a longer code to almost guarantee uniqueness
	return utils.GenerateShortCode(10)
}
