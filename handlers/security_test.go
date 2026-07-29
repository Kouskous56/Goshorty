package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"goshorty/storage"
)

func TestRateLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewRateLimiter()
	router := gin.New()
	router.GET("/", limiter.Limit("test", 2, time.Minute), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for i, expected := range []int{http.StatusNoContent, http.StatusNoContent, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != expected {
			t.Fatalf("request %d: expected %d, got %d", i+1, expected, w.Code)
		}
		if i == 2 && w.Header().Get("Retry-After") == "" {
			t.Fatal("expected Retry-After header")
		}
	}
}

type testHealthChecker struct {
	err error
}

func (c testHealthChecker) Ping(_ context.Context) error {
	return c.err
}

func TestReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		checker  storage.HealthChecker
		expected int
	}{
		{"ready without external dependency", nil, http.StatusOK},
		{"ready database", testHealthChecker{}, http.StatusOK},
		{"database unavailable", testHealthChecker{err: errors.New("offline")}, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			handler := NewHandlerWithHealth(nil, tt.checker)
			router.GET("/ready", handler.Ready)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
			if w.Code != tt.expected {
				t.Fatalf("expected %d, got %d", tt.expected, w.Code)
			}
		})
	}
}
