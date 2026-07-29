package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(securityHeadersMiddleware())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	for _, header := range []string{
		"Content-Security-Policy",
		"Referrer-Policy",
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Permissions-Policy",
		"Strict-Transport-Security",
	} {
		if w.Header().Get(header) == "" {
			t.Errorf("expected %s header", header)
		}
	}
}

func TestCORSAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(corsMiddleware([]string{"https://short.example.com"}))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	allowedReq := httptest.NewRequest(http.MethodGet, "/", nil)
	allowedReq.Header.Set("Origin", "https://short.example.com")
	allowedW := httptest.NewRecorder()
	router.ServeHTTP(allowedW, allowedReq)
	if allowedW.Header().Get("Access-Control-Allow-Origin") != "https://short.example.com" {
		t.Fatal("expected allowed origin in response")
	}

	blockedReq := httptest.NewRequest(http.MethodOptions, "/", nil)
	blockedReq.Header.Set("Origin", "https://evil.example.com")
	blockedW := httptest.NewRecorder()
	router.ServeHTTP(blockedW, blockedReq)
	if blockedW.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden preflight, got %d", blockedW.Code)
	}
}

func TestRequestBodyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestBodyLimitMiddleware(8))
	router.POST("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(strings.Repeat("x", 9)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
}
