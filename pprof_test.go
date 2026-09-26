package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"goshorty/config"
)

// hasPprofRoutes returns the registered /debug/pprof routes.
func hasPprofRoutes(router *gin.Engine) []string {
	var found []string
	for _, rt := range router.Routes() {
		if strings.HasPrefix(rt.Path, "/debug/pprof") {
			found = append(found, rt.Method+" "+rt.Path)
		}
	}
	return found
}

// TestPprofSurfaceServesProfiles mounts registerPprof directly and asserts the
// built-in endpoints answer 200 with pprof content.
func TestPprofSurfaceServesProfiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerPprof(router)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /debug/pprof/: expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/debug/pprof/") {
		t.Errorf("index page should list pprof endpoints, got %q", w.Body.String())
	}

	for _, path := range []string{
		"/debug/pprof/cmdline",
		"/debug/pprof/symbol",
		"/debug/pprof/goroutine",
		"/debug/pprof/heap",
		"/debug/pprof/allocs",
		"/debug/pprof/block",
		"/debug/pprof/mutex",
		"/debug/pprof/threadcreate",
	} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s: expected 200, got %d", path, w.Code)
		}
	}
	// /debug/pprof/profile (30s CPU sample) and /debug/pprof/trace (1s) are
	// intentionally not exercised: they block for their sampling duration.
}

// TestPprofOptInThroughProductionBuilder proves the real router (as built by
// newAppRouter, the same assembly main() uses) exposes the profiling surface
// only when PPROF_ENABLED is set. Route presence is checked via Routes() — the
// SPA fallback answers unknown non-/api paths with 200, so status codes alone
// cannot prove absence.
func TestPprofOptInThroughProductionBuilder(t *testing.T) {
	router, _ := newTestAPIRouter()
	if found := hasPprofRoutes(router); len(found) > 0 {
		t.Fatalf("pprof must be off by default, found routes: %v", found)
	}

	cfg := config.NewConfig()
	cfg.Ops.PprofEnabled = true
	enabled, _ := newTestAPIRouterCfg(cfg)
	if found := hasPprofRoutes(enabled); len(found) == 0 {
		t.Fatal("PPROF_ENABLED router must register /debug/pprof routes")
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/debug/pprof/heap", nil)
	enabled.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PPROF_ENABLED router should serve /debug/pprof/heap: got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/octet-stream") {
		t.Errorf("expected an octet-stream heap profile, got Content-Type %q", ct)
	}
}
