package models

import "time"

// ShortenRequest is the request for creating a short URL
type ShortenRequest struct {
	URL        string `json:"url" binding:"required,url"`
	ExpiresIn  string `json:"expires_in" binding:"omitempty,oneof=5m 15m 1h 24h 168h"`
	CustomCode string `json:"custom_code" binding:"omitempty,alphanum,max=50"`
}

// URLData represents a shortened URL stored in the system
type URLData struct {
	ID          string    `json:"id"`
	ShortCode   string    `json:"short_code"`
	OriginalURL string    `json:"original_url"`
	ExpiresIn   string    `json:"expires_in"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by"`
	Visits      int64     `json:"visits"`
}

// ShortenResponse is the response after creating a short URL
type ShortenResponse struct {
	ID          string    `json:"id"`
	ShortURL    string    `json:"short_url"`
	ShortCode   string    `json:"short_code"`
	OriginalURL string    `json:"original_url"`
	ExpiresIn   string    `json:"expires_in"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// ErrorResponse is the standard error response
type ErrorResponse struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}
