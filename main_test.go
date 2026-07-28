package main

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"goshorty/config"
	"goshorty/handlers"
	"goshorty/models"
	"goshorty/services"
	"goshorty/storage"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestCreateShortURL tests URL shortening functionality
func TestCreateShortURL(t *testing.T) {
	// Setup
	cfg := config.NewConfig()
	store := storage.NewStorage()
	service := services.NewURLService(store, cfg)
	h := handlers.NewHandler(service)

	// Create Gin router for testing
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-user-id")
		c.Set("role", "admin")
		c.Next()
	})
	router.POST("/api/shorten", h.CreateShortURL)

	// Test cases
	tests := []struct {
		name           string
		request        models.ShortenRequest
		expectedStatus int
	}{
		{
			name: "Valid URL with default TTL",
			request: models.ShortenRequest{
				URL: "https://www.google.com",
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "Valid URL with custom TTL",
			request: models.ShortenRequest{
				URL:       "https://www.example.com",
				ExpiresIn: "1h",
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "Valid URL with custom code",
			request: models.ShortenRequest{
				URL:        "https://www.github.com",
				ExpiresIn:  "24h",
				CustomCode: "github",
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "Invalid URL",
			request: models.ShortenRequest{
				URL: "not-a-url",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Empty URL",
			request: models.ShortenRequest{
				URL: "",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req, _ := http.NewRequest("POST", "/api/shorten", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusCreated {
				var response models.ShortenResponse
				json.Unmarshal(w.Body.Bytes(), &response)
				if response.ShortCode == "" {
					t.Error("Expected short code in response")
				}
			}
		})
	}
}

// TestTTLExpiration tests that URLs expire correctly
func TestTTLExpiration(t *testing.T) {
	cfg := config.NewConfig()
	store := storage.NewStorage()
	service := services.NewURLService(store, cfg)

	// Create a URL with very short TTL
	cfg.TTL.Options["test"] = 100 * time.Millisecond

	request := &models.ShortenRequest{
		URL:       "https://example.com",
		ExpiresIn: "test",
	}

	response, err := service.CreateShortURL(request, "test-user")
	if err != nil {
		t.Fatalf("Failed to create short URL: %v", err)
	}

	// URL should be retrievable immediately
	_, err = service.GetOriginalURL(response.ShortCode)
	if err != nil {
		t.Error("URL should be retrievable immediately after creation")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// URL should no longer be retrievable
	_, err = service.GetOriginalURL(response.ShortCode)
	if err == nil {
		t.Error("URL should be expired after TTL")
	}
}

// TestVisitTracking tests visit count increment
func TestVisitTracking(t *testing.T) {
	cfg := config.NewConfig()
	store := storage.NewStorage()
	service := services.NewURLService(store, cfg)

	request := &models.ShortenRequest{
		URL: "https://example.com",
	}

	response, _ := service.CreateShortURL(request, "test-user")

	// Access URL multiple times
	for i := 0; i < 5; i++ {
		service.GetOriginalURL(response.ShortCode)
	}

	// Check visit count
	urlInfo, _ := service.GetURLInfo(response.ShortCode, "test-user", models.RoleUser)
	if urlInfo.Visits != 5 {
		t.Errorf("Expected 5 visits, got %d", urlInfo.Visits)
	}
}

// TestCustomCode tests custom short code functionality
func TestCustomCode(t *testing.T) {
	cfg := config.NewConfig()
	store := storage.NewStorage()
	service := services.NewURLService(store, cfg)

	// Create first URL with custom code
	request1 := &models.ShortenRequest{
		URL:        "https://example.com",
		CustomCode: "custom",
	}
	response1, err := service.CreateShortURL(request1, "test-user")
	if err != nil {
		t.Fatalf("Failed to create URL with custom code: %v", err)
	}

	if response1.ShortCode != "custom" {
		t.Errorf("Expected short code 'custom', got %s", response1.ShortCode)
	}

	// Try to create another URL with same custom code
	request2 := &models.ShortenRequest{
		URL:        "https://example2.com",
		CustomCode: "custom",
	}
	_, err = service.CreateShortURL(request2, "test-user")
	if err == nil {
		t.Error("Should not allow duplicate custom code")
	}
}

// TestStatistics tests statistics gathering
func TestStatistics(t *testing.T) {
	cfg := config.NewConfig()
	store := storage.NewStorage()
	service := services.NewURLService(store, cfg)

	// Create multiple URLs
	for i := 0; i < 3; i++ {
		request := &models.ShortenRequest{
			URL: "https://example.com",
		}
		service.CreateShortURL(request, "test-user")
	}

	stats, err := service.GetStats("test-user", models.RoleUser)
	if err != nil {
		t.Fatalf("Failed to load stats: %v", err)
	}
	if stats["total_urls"] != 3 {
		t.Errorf("Expected 3 URLs, got %v", stats["total_urls"])
	}
}
