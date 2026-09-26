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
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

// --- T4: /api/v1 canonical surface + backward-compatible aliases ---

// newTestAPIRouter builds a router with the same wiring as production
// (registerAPIGroup for /api and /api/v1, health aliases, redirect routes and
// the NoRoute fallback) for route-layer tests.
func newTestAPIRouter() (*gin.Engine, *services.URLService) {
	gin.SetMode(gin.TestMode)
	cfg := config.NewConfig()

	urlStore := storage.NewStorage()
	urlService := services.NewURLService(urlStore, cfg)

	userStore, err := storage.NewUserStorage("test-admin", "admin@goshorty.local")
	if err != nil {
		panic(err)
	}
	tokenService := services.NewTokenService("test-secret-key")
	authHandler := handlers.NewAuthHandler(userStore, tokenService)
	h := handlers.NewHandler(urlService)

	// Build the exact production route surface through the same builder main()
	// uses, so every main-level test exercises the real wiring (observability,
	// CORS, security headers, body limits, static assets) instead of a
	// hand-rolled twin that could drift from production.
	router, err := newAppRouter(cfg, slog.New(slog.DiscardHandler), h, authHandler, NewHTTPMetrics(), appRouterAssets{version: "test-build"})
	if err != nil {
		panic(err)
	}
	return router, urlService
}

// loginAsAdmin returns a valid bearer token for the bootstrap admin user.
func loginAsAdmin(t *testing.T, router *gin.Engine) string {
	t.Helper()
	body, _ := json.Marshal(models.LoginRequest{Username: "admin", Password: "test-admin"})
	req, _ := http.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin login: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var resp models.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("admin login: parse response: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("admin login: empty token")
	}
	return resp.Token
}

// TestV1AndLegacyAuthRoutesWork proves /api/v1/auth/* registers the same
// handlers as the legacy /api/auth/* and both surfaces are live.
func TestV1AndLegacyAuthRoutesWork(t *testing.T) {
	router, _ := newTestAPIRouter()

	register := func(path, username string) int {
		t.Helper()
		body, _ := json.Marshal(models.RegisterRequest{
			Username: username,
			Password: "secure-password-123",
			Email:    username + "@example.com",
		})
		req, _ := http.NewRequest(http.MethodPost, path, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	if code := register("/api/v1/auth/register", "v1user"); code != http.StatusCreated {
		t.Fatalf("v1 register: expected 201, got %d", code)
	}
	if code := register("/api/auth/register", "legacyuser"); code != http.StatusCreated {
		t.Fatalf("legacy register: expected 201, got %d", code)
	}

	// Login through both surfaces.
	login := func(path, username string) string {
		t.Helper()
		body, _ := json.Marshal(models.LoginRequest{Username: username, Password: "secure-password-123"})
		req, _ := http.NewRequest(http.MethodPost, path, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("login %s: expected 200, got %d (%s)", path, w.Code, w.Body.String())
		}
		var resp models.LoginResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("login %s: parse: %v", path, err)
		}
		if resp.Token == "" {
			t.Fatalf("login %s: empty token", path)
		}
		return resp.Token
	}

	v1Token := login("/api/v1/auth/login", "v1user")
	legacyToken := login("/api/auth/login", "legacyuser")

	me := func(path, token string) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}
	if code := me("/api/v1/auth/me", v1Token); code != http.StatusOK {
		t.Fatalf("v1 /auth/me: expected 200, got %d", code)
	}
	if code := me("/api/auth/me", legacyToken); code != http.StatusOK {
		t.Fatalf("legacy /auth/me: expected 200, got %d", code)
	}
}

// TestURLAliasesShareHandlers proves the canonical /api/v1/urls* routes and
// the legacy /api/shorten* aliases hit the same handlers, and that operations
// through one surface are visible through the other.
func TestURLAliasesShareHandlers(t *testing.T) {
	router, _ := newTestAPIRouter()
	token := loginAsAdmin(t, router)
	auth := "Bearer " + token

	create := func(path, url, customCode string) models.ShortenResponse {
		t.Helper()
		body, _ := json.Marshal(models.ShortenRequest{URL: url, CustomCode: customCode})
		req, _ := http.NewRequest(http.MethodPost, path, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d (%s)", path, w.Code, w.Body.String())
		}
		var resp models.ShortenResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("create %s: parse: %v", path, err)
		}
		return resp
	}

	v1 := create("/api/v1/urls", "https://example.com/v1", "t4v1")
	if !strings.HasSuffix(v1.ShortURL, "/r/"+v1.ShortCode) {
		t.Fatalf("v1 public link does not use /r/:code format: %s", v1.ShortURL)
	}
	legacy := create("/api/shorten", "https://example.com/legacy", "t4leg")

	get := func(path string) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	// Info and list on both surfaces.
	for _, path := range []string{
		"/api/v1/urls/" + v1.ShortCode,
		"/api/v1/urls/" + legacy.ShortCode,
		"/api/shorten/" + v1.ShortCode,
		"/api/shorten/" + legacy.ShortCode,
		"/api/v1/urls",
		"/api/shorten/all",
		"/api/v1/stats",
		"/api/stats",
		"/api/v1/auth/users",
		"/api/auth/users",
	} {
		if code := get(path); code != http.StatusOK {
			t.Errorf("GET %s: expected 200, got %d", path, code)
		}
	}

	// Delete via v1, then confirm the legacy info route reports it gone.
	del := func(path string) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodDelete, path, nil)
		req.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}
	if code := del("/api/v1/urls/" + v1.ShortCode); code != http.StatusOK {
		t.Fatalf("DELETE v1: expected 200, got %d", code)
	}
	if code := get("/api/shorten/" + v1.ShortCode); code != http.StatusNotFound {
		t.Fatalf("legacy info after v1 delete: expected 404, got %d", code)
	}
	if code := del("/api/shorten/" + legacy.ShortCode); code != http.StatusOK {
		t.Fatalf("DELETE legacy: expected 200, got %d", code)
	}
	if code := get("/api/v1/urls/" + legacy.ShortCode); code != http.StatusNotFound {
		t.Fatalf("v1 info after legacy delete: expected 404, got %d", code)
	}
}

// TestRedirectRoutes proves the canonical /r/:code and the backward-compatible
// aliases all redirect to the original URL.
func TestRedirectRoutes(t *testing.T) {
	router, urlService := newTestAPIRouter()

	created, err := urlService.CreateShortURL(&models.ShortenRequest{
		URL:        "https://example.com/target",
		CustomCode: "t4redir",
	}, "owner-id")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	code := created.ShortCode

	redirect := func(path string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code, w.Header().Get("Location")
	}

	for _, path := range []string{"/r/" + code, "/s/" + code, "/goshorty/1h/" + code} {
		status, loc := redirect(path)
		if status != http.StatusFound {
			t.Errorf("GET %s: expected 302, got %d", path, status)
		}
		if loc != "https://example.com/target" {
			t.Errorf("GET %s: expected target Location, got %q", path, loc)
		}
	}
}

// TestHealthAndReadyAliases proves the canonical and legacy health/ready
// endpoints all respond with 200.
func TestHealthAndReadyAliases(t *testing.T) {
	router, _ := newTestAPIRouter()
	for _, path := range []string{"/health", "/health/live", "/ready", "/health/ready"} {
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s: expected 200, got %d (%s)", path, w.Code, w.Body.String())
		}
	}
}

// TestAPIV1UnknownRouteReturnsJSON404 proves unknown /api paths get a JSON 404
// (not the SPA fallback) while non-API paths still serve the SPA index.
func TestAPIV1UnknownRouteReturnsJSON404(t *testing.T) {
	router, _ := newTestAPIRouter()
	for _, path := range []string{"/api/v1/does-not-exist", "/api/does-not-exist"} {
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s: expected 404, got %d", path, w.Code)
		}
		if !json.Valid(w.Body.Bytes()) {
			t.Errorf("GET %s: expected JSON body, got %q", path, w.Body.String())
		}
	}

	req, _ := http.NewRequest(http.MethodGet, "/some/spa/route", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("SPA fallback: expected 200, got %d", w.Code)
	}
}

// TestAPIInfoEndpoints proves /api and /api/v1 describe the API surface and
// that /api points at /api/v1 as the canonical version.
func TestAPIInfoEndpoints(t *testing.T) {
	router, _ := newTestAPIRouter()

	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api: expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "\"canonical_api\":\"/api/v1\"") {
		t.Errorf("GET /api: expected canonical_api field, got %s", w.Body.String())
	}

	req, _ = http.NewRequest(http.MethodGet, "/api/v1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1: expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "\"POST /api/v1/urls\"") {
		t.Errorf("GET /api/v1: expected v1 endpoints listing, got %s", w.Body.String())
	}
}

// TestV1PaginatedListsAndLegacyShapeUntouched proves the canonical v1 list
// endpoints paginate (limit + cursor keyset, total, sorted newest first)
// while the legacy aliases keep their original response shapes.
func TestV1PaginatedListsAndLegacyShapeUntouched(t *testing.T) {
	router, urlService := newTestAPIRouter()
	token := loginAsAdmin(t, router)
	auth := "Bearer " + token

	for _, code := range []string{"p1", "p2", "p3"} {
		if _, err := urlService.CreateShortURL(&models.ShortenRequest{
			URL:        "https://example.com/" + code,
			CustomCode: code,
		}, "test-owner"); err != nil {
			t.Fatalf("create %s: %v", code, err)
		}
	}

	fetch := func(path string) (int, map[string]interface{}) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			return w.Code, nil
		}
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		return w.Code, body
	}

	// First v1 page: 1 URL, total 3, next_cursor present.
	status, body := fetch("/api/v1/urls?limit=1")
	if status != http.StatusOK {
		t.Fatalf("v1 list: expected 200, got %d", status)
	}
	if body["total"] != float64(3) {
		t.Errorf("total = %v, want 3", body["total"])
	}
	urls, _ := body["urls"].([]interface{})
	if len(urls) != 1 {
		t.Fatalf("page size = %d, want 1", len(urls))
	}
	next, _ := body["next_cursor"].(string)
	if next == "" {
		t.Fatal("expected next_cursor on first page")
	}

	// Walk remaining pages with the cursor until exhausted.
	seen := map[string]bool{}
	seen[urls[0].(map[string]interface{})["short_code"].(string)] = true
	cursor := next
	for cursor != "" {
		status, body = fetch("/api/v1/urls?limit=1&cursor=" + cursor)
		if status != http.StatusOK {
			t.Fatalf("walk page: expected 200, got %d", status)
		}
		for _, raw := range body["urls"].([]interface{}) {
			seen[raw.(map[string]interface{})["short_code"].(string)] = true
		}
		next, _ = body["next_cursor"].(string)
		cursor = next
	}
	if len(seen) != 3 {
		t.Errorf("walked %d unique codes, want 3", len(seen))
	}

	// Bad limit and bad cursor are 400 JSON errors on v1.
	if status, _ := fetch("/api/v1/urls?limit=abc"); status != http.StatusBadRequest {
		t.Errorf("bad limit: expected 400, got %d", status)
	}
	if status, _ := fetch("/api/v1/urls?cursor=!!!bad"); status != http.StatusBadRequest {
		t.Errorf("bad cursor: expected 400, got %d", status)
	}

	// Legacy alias keeps its original shape: only "urls".
	status, body = fetch("/api/shorten/all")
	if status != http.StatusOK {
		t.Fatalf("legacy list: expected 200, got %d", status)
	}
	if _, ok := body["next_cursor"]; ok {
		t.Error("legacy list must not expose next_cursor")
	}
	if _, ok := body["total"]; ok {
		t.Error("legacy list must not expose total")
	}
	if legacyURLs, _ := body["urls"].([]interface{}); len(legacyURLs) != 3 {
		t.Errorf("legacy list size = %d, want 3", len(legacyURLs))
	}

	// Users: v1 paginated shape, legacy original shape.
	status, body = fetch("/api/v1/auth/users")
	if status != http.StatusOK {
		t.Fatalf("v1 users: expected 200, got %d", status)
	}
	if body["total"] == nil {
		t.Error("v1 users must expose total")
	}
	if _, ok := body["users"]; !ok {
		t.Error("v1 users must expose users array")
	}
	status, body = fetch("/api/auth/users")
	if status != http.StatusOK {
		t.Fatalf("legacy users: expected 200, got %d", status)
	}
	if _, ok := body["total"]; ok {
		t.Error("legacy users must not expose total")
	}
}
