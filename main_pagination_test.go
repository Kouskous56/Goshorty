package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"goshorty/models"
)

// TestV1UsersPaginatedAndLegacyShape proves the canonical v1 auth/users
// endpoint paginates (keyset cursor, username ASC, total) while the legacy
// /api/auth/users alias keeps its full-list response shape.
func TestV1UsersPaginatedAndLegacyShape(t *testing.T) {
	router, _ := newTestAPIRouter()
	token := loginAsAdmin(t, router)
	auth := "Bearer " + token

	register := func(username string) {
		t.Helper()
		body, _ := json.Marshal(models.RegisterRequest{
			Username: username,
			Password: "password-" + username,
			Email:    username + "@test.local",
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("register %s: expected 201, got %d (%s)", username, w.Code, w.Body.String())
		}
	}
	register("zeta")
	register("alpha")
	register("bravo")

	get := func(path string) map[string]interface{} {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d (%s)", path, w.Code, w.Body.String())
		}
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		return body
	}

	usernames := func(raw []interface{}) []string {
		out := make([]string, 0, len(raw))
		for _, u := range raw {
			out = append(out, u.(map[string]interface{})["username"].(string))
		}
		return out
	}

	// Page 1: 2 of the 4 users (admin + alpha + bravo + zeta), username ASC.
	first := get("/api/v1/auth/users?limit=2")
	if n, _ := first["total"].(float64); n != 4 {
		t.Fatalf("total = %v, want 4", first["total"])
	}
	next, _ := first["next_cursor"].(string)
	if next == "" {
		t.Fatal("first page missing next_cursor")
	}
	got := usernames(first["users"].([]interface{}))
	if len(got) != 2 || got[0] != "admin" || got[1] != "alpha" {
		t.Fatalf("page 1 = %v, want [admin alpha]", got)
	}

	// Walk the remaining pages until the cursor is exhausted.
	var rest []string
	cursor := next
	for cursor != "" {
		body := get("/api/v1/auth/users?limit=2&cursor=" + cursor)
		rest = append(rest, usernames(body["users"].([]interface{}))...)
		cursor, _ = body["next_cursor"].(string)
	}
	if strings.Join(rest, ",") != "bravo,zeta" {
		t.Fatalf("remaining pages = %v, want [bravo zeta]", rest)
	}

	// Bad cursors are rejected on the v1 surface.
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/auth/users?cursor=!!!bad", nil)
	req.Header.Set("Authorization", auth)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad cursor: expected 400, got %d", w.Code)
	}

	// Legacy alias: full list, no cursor token.
	legacy := get("/api/auth/users")
	if _, ok := legacy["next_cursor"]; ok {
		t.Error("legacy list must not expose next_cursor")
	}
	if n := len(legacy["users"].([]interface{})); n != 4 {
		t.Errorf("legacy users count = %d, want 4", n)
	}
}
