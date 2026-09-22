package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func readEmbeddedStatic(t *testing.T, name string) string {
	t.Helper()
	data, err := staticFiles.ReadFile(name)
	if err != nil {
		t.Fatalf("embedded %s is missing: %v", name, err)
	}
	return string(data)
}

// TestIndexHTMLHasNoInlineScripts guards the P2.5 refactor: the SPA must not
// inline scripts or event handler attributes so the strict CSP
// (script-src 'self') stays effective.
func TestIndexHTMLHasNoInlineScripts(t *testing.T) {
	html := readEmbeddedStatic(t, "static/index.html")

	if n := strings.Count(html, "<script"); n != 1 {
		t.Errorf("index.html must contain exactly one external <script> tag, found %d", n)
	}
	if !strings.Contains(html, `<script src="/static/js/app.js"`) {
		t.Error("index.html must load /static/js/app.js")
	}
	if !strings.Contains(html, `<link rel="stylesheet" href="/static/css/style.css"`) {
		t.Error("index.html must load /static/css/style.css")
	}
	if strings.Contains(html, "onclick=") {
		t.Error("index.html must not contain inline onclick handlers")
	}
}

// TestAppJSUsesCanonicalAPIAndSessionRestore verifies the SPA targets the
// canonical /api/v1 surface and re-validates the stored token on load.
func TestAppJSUsesCanonicalAPIAndSessionRestore(t *testing.T) {
	js := readEmbeddedStatic(t, "static/js/app.js")

	if !strings.Contains(js, "'/api/v1'") {
		t.Error("app.js must target the canonical /api/v1 API base")
	}
	for _, needle := range []string{
		"restoreSession",
		"/auth/me",
		"forceSessionExpired",
		"data-action",
	} {
		if !strings.Contains(js, needle) {
			t.Errorf("app.js missing %q (session validation / event wiring)", needle)
		}
	}
	if strings.Contains(js, "onclick") {
		t.Error("app.js must not generate inline onclick handlers")
	}
}

// TestStyleCSSPresent makes sure the extracted stylesheet is embedded and
// still carries the SPA layout rules.
func TestStyleCSSPresent(t *testing.T) {
	css := readEmbeddedStatic(t, "static/css/style.css")
	for _, sel := range []string{".auth-section", ".dashboard", ".tab-button", "@media", ".form-group select"} {
		if !strings.Contains(css, sel) {
			t.Errorf("style.css missing %q rule", sel)
		}
	}
}

// TestStaticAssetsServed hits the asset routes on the shared test router
// (which mounts the embedded static FS like production) and checks the bodies.
//
// Note: /static/index.html is intentionally NOT tested here — net/http's
// FileServer 301-redirects every request ending in /index.html to "./"
// (golang.org/issue/11857). The SPA root is served from "/" by the NoRoute
// fallback instead, which is already covered by the SPA fallback test.
func TestStaticAssetsServed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, _ := newTestAPIRouter()

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/static/js/app.js", want: "restoreSession"},
		{path: "/static/css/style.css", want: ".auth-section"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s: expected 200, got %d", tc.path, w.Code)
			continue
		}
		if body := w.Body.String(); !strings.Contains(body, tc.want) {
			t.Errorf("GET %s: body missing %q", tc.path, tc.want)
		}
	}
}
