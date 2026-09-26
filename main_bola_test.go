package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"goshorty/models"
)

// loginAs returns a fresh bearer token for username/password against the v1
// auth login surface (exercising the canonical routing as well).
func loginAs(t *testing.T, router *gin.Engine, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(models.LoginRequest{Username: username, Password: password})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login %s: expected 200, got %d (%s)", username, w.Code, w.Body.String())
	}
	var resp models.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("login %s: parse response: %v", username, err)
	}
	if resp.Token == "" {
		t.Fatalf("login %s: empty token", username)
	}
	return resp.Token
}

// TestBOLACrossUserAccessIsOpaque is the end-to-end authorization contract:
// a non-admin user can never learn about or delete another user's URL through
// the API (both read and delete answer 404, not 403, so existence is not
// leaked), while the owner, an admin, and the public redirect keep their
// legitimate access.
func TestBOLACrossUserAccessIsOpaque(t *testing.T) {
	router, _ := newTestAPIRouter()

	register := func(username, password string) string {
		t.Helper()
		body, _ := json.Marshal(models.RegisterRequest{
			Username: username,
			Password: password,
			Email:    username + "@test.local",
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("register %s: expected 201, got %d (%s)", username, w.Code, w.Body.String())
		}
		return loginAs(t, router, username, password)
	}

	aliceToken := register("alice", "password-alice-1")
	bobToken := register("bob", "password-bob-1")
	adminToken := loginAsAdmin(t, router)

	do := func(method, path string, body []byte, token string) *httptest.ResponseRecorder {
		t.Helper()
		var reader io.Reader
		if body != nil {
			reader = bytes.NewBuffer(body)
		}
		req, _ := http.NewRequest(method, path, reader)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Alice creates a URL with a known code.
	createBody, _ := json.Marshal(models.ShortenRequest{
		URL:        "https://example.com/alice-private",
		CustomCode: "alice1",
	})
	if w := do(http.MethodPost, "/api/v1/urls", createBody, aliceToken); w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d (%s)", w.Code, w.Body.String())
	}

	// The protected surface rejects anonymous sessions.
	if w := do(http.MethodGet, "/api/v1/urls/alice1", nil, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous info: expected 401, got %d", w.Code)
	}

	// The redirect is public by design (TTL-gated only).
	if w := do(http.MethodGet, "/r/alice1", nil, ""); w.Code != http.StatusFound {
		t.Errorf("anonymous redirect: expected 302, got %d", w.Code)
	}

	// Bob learns nothing and breaks nothing: identical opaque 404s.
	if w := do(http.MethodGet, "/api/v1/urls/alice1", nil, bobToken); w.Code != http.StatusNotFound {
		t.Errorf("bob GET: expected 404, got %d", w.Code)
	}
	if w := do(http.MethodDelete, "/api/v1/urls/alice1", nil, bobToken); w.Code != http.StatusNotFound {
		t.Errorf("bob DELETE: expected 404, got %d", w.Code)
	}

	// Alice and the admin still see the URL — bob's attempts had no effect.
	if w := do(http.MethodGet, "/api/v1/urls/alice1", nil, aliceToken); w.Code != http.StatusOK {
		t.Errorf("alice GET: expected 200, got %d", w.Code)
	}
	if w := do(http.MethodGet, "/api/v1/urls/alice1", nil, adminToken); w.Code != http.StatusOK {
		t.Errorf("admin GET: expected 200, got %d", w.Code)
	}

	// List totals stay owner-scoped.
	var aliceList map[string]interface{}
	if w := do(http.MethodGet, "/api/v1/urls", nil, aliceToken); w.Code == http.StatusOK {
		_ = json.Unmarshal(w.Body.Bytes(), &aliceList)
	}
	if total, _ := aliceList["total"].(float64); total != 1 {
		t.Errorf("alice list total = %v, want 1", aliceList["total"])
	}
	var bobList map[string]interface{}
	if w := do(http.MethodGet, "/api/v1/urls", nil, bobToken); w.Code == http.StatusOK {
		_ = json.Unmarshal(w.Body.Bytes(), &bobList)
	}
	if total, _ := bobList["total"].(float64); total != 0 {
		t.Errorf("bob list total = %v, want 0", bobList["total"])
	}

	// Alice deletes her own URL; afterwards nobody can read it and the
	// redirect stops resolving.
	if w := do(http.MethodDelete, "/api/v1/urls/alice1", nil, aliceToken); w.Code != http.StatusOK {
		t.Errorf("alice DELETE: expected 200, got %d", w.Code)
	}
	for name, tok := range map[string]string{"bob": bobToken, "alice": aliceToken, "admin": adminToken} {
		if w := do(http.MethodGet, "/api/v1/urls/alice1", nil, tok); w.Code != http.StatusNotFound {
			t.Errorf("%s GET after delete: expected 404, got %d", name, w.Code)
		}
	}
	if w := do(http.MethodGet, "/r/alice1", nil, ""); w.Code != http.StatusNotFound {
		t.Errorf("redirect after delete: expected 404, got %d", w.Code)
	}
}
