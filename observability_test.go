package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestObservabilityUsesRouteTemplatesAndRequestIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	metrics := NewHTTPMetrics()
	router := gin.New()
	router.Use(observabilityMiddleware(logger, metrics))
	router.GET("/items/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/metrics", metrics.Handler)

	request := httptest.NewRequest(http.MethodGet, "/items/private-code", nil)
	request.Header.Set(requestIDHeader, "request-1234")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Header().Get(requestIDHeader) != "request-1234" {
		t.Fatalf("request ID was not propagated")
	}
	if strings.Contains(logs.String(), "private-code") ||
		!strings.Contains(logs.String(), `"route":"/items/:id"`) {
		t.Fatalf("log must contain route template without raw path: %s", logs.String())
	}

	metricsResponse := httptest.NewRecorder()
	router.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricsResponse.Body.String()
	if !strings.Contains(body, `route="/items/:id"`) || strings.Contains(body, "private-code") {
		t.Fatalf("unexpected metrics labels: %s", body)
	}
}

func TestObservabilityReplacesInvalidRequestIDAndRecovers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	metrics := NewHTTPMetrics()
	router := gin.New()
	router.Use(observabilityMiddleware(logger, metrics), structuredRecovery(logger))
	router.GET("/panic", func(_ *gin.Context) { panic("sensitive failure") })

	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	request.Header.Set(requestIDHeader, "bad\nvalue")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", response.Code)
	}
	if id := response.Header().Get(requestIDHeader); !validRequestID.MatchString(id) {
		t.Fatalf("invalid generated request ID: %q", id)
	}
	if !strings.Contains(logs.String(), `"msg":"http_panic"`) {
		t.Fatalf("missing structured panic log: %s", logs.String())
	}
}

func TestMetricsAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/metrics", metricsAuthMiddleware("a-strong-operational-token"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without metrics token, got %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("Authorization", "Bearer a-strong-operational-token")
	authorized := httptest.NewRecorder()
	router.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusNoContent {
		t.Fatalf("expected authorized metrics request, got %d", authorized.Code)
	}
}
