package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// fetchOpenAPISpecGET GETs /api/v1/openapi.json from the test router and
// asserts it is served as valid JSON. It returns the parsed document.
func fetchOpenAPISpec(t *testing.T, router *gin.Engine) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/openapi.json: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var spec map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("spec body is not valid JSON: %v", err)
	}
	return spec
}

// TestOpenAPISpecServed checks the document is reachable and has the top-level
// OpenAPI 3.1 shape.
func TestOpenAPISpecServed(t *testing.T) {
	router, _ := newTestAPIRouter()
	spec := fetchOpenAPISpec(t, router)

	if v, _ := spec["openapi"].(string); !strings.HasPrefix(v, "3.1") {
		t.Errorf("openapi version = %q, want 3.1.x", v)
	}
	info, _ := spec["info"].(map[string]any)
	if info == nil {
		t.Fatal("spec info missing")
	}
	if title, _ := info["title"].(string); title == "" {
		t.Error("spec info.title missing")
	}
	paths, _ := spec["paths"].(map[string]any)
	if len(paths) == 0 {
		t.Fatal("spec paths is empty")
	}
}

// TestOpenAPISpecCoversCanonicalSurface asserts every route the canonical
// surface actually registers (plus public redirect and operational routes) is
// described in the spec.
func TestOpenAPISpecCoversCanonicalSurface(t *testing.T) {
	router, _ := newTestAPIRouter()
	spec := fetchOpenAPISpec(t, router)
	paths, _ := spec["paths"].(map[string]any)

	want := []string{
		"/api/v1/auth/register",
		"/api/v1/auth/login",
		"/api/v1/auth/me",
		"/api/v1/auth/password",
		"/api/v1/auth/revoke",
		"/api/v1/urls",
		"/api/v1/urls/{code}",
		"/api/v1/stats",
		"/api/v1/auth/users",
		"/api/v1/auth/users/{username}/role",
		"/api/v1/auth/users/{username}",
		"/r/{code}",
		"/s/{code}",
		"/goshorty/{timeout}/{code}",
		"/health",
		"/health/live",
		"/ready",
		"/health/ready",
		"/version",
		"/metrics",
		"/api/v1/openapi.json",
	}
	for _, p := range want {
		if _, ok := paths[p]; !ok {
			t.Errorf("spec missing canonical path %q", p)
		}
	}
}

// TestOpenAPISpecOperationsHaveResponses checks every path item contains at
// least one recognized operation and each operation declares responses.
func TestOpenAPISpecOperationsHaveResponses(t *testing.T) {
	router, _ := newTestAPIRouter()
	spec := fetchOpenAPISpec(t, router)
	paths, _ := spec["paths"].(map[string]any)

	for path, item := range paths {
		m, ok := item.(map[string]any)
		if !ok {
			t.Errorf("path %q: expected object, got %T", path, item)
			continue
		}
		found := false
		for _, method := range []string{"get", "post", "put", "delete"} {
			op, ok := m[method].(map[string]any)
			if !ok {
				continue
			}
			found = true
			if resp, ok := op["responses"].(map[string]any); !ok || len(resp) == 0 {
				t.Errorf("%s %s: operation missing responses", strings.ToUpper(method), path)
			}
		}
		if !found {
			t.Errorf("path %q has no recognized operation (get/post/put/delete)", path)
		}
	}
}

// TestOpenAPISpecMarksLegacyAliasesDeprecated asserts the backward-compatible
// /api/* and legacy redirect aliases are flagged deprecated so tooling does
// not suggest them for new integrations.
func TestOpenAPISpecMarksLegacyAliasesDeprecated(t *testing.T) {
	router, _ := newTestAPIRouter()
	spec := fetchOpenAPISpec(t, router)
	paths, _ := spec["paths"].(map[string]any)

	legacy := []string{
		"/api",
		"/api/auth/login",
		"/api/auth/register",
		"/api/auth/me",
		"/api/auth/password",
		"/api/auth/revoke",
		"/api/shorten",
		"/api/shorten/{code}",
		"/api/shorten/all",
		"/api/stats",
		"/api/auth/users",
		"/api/auth/users/{username}/role",
		"/api/auth/users/{username}",
		"/s/{code}",
		"/goshorty/{timeout}/{code}",
	}
	for _, p := range legacy {
		item, ok := paths[p].(map[string]any)
		if !ok {
			t.Errorf("legacy path %q missing from spec", p)
			continue
		}
		deprecated := false
		for _, method := range []string{"get", "post", "put", "delete"} {
			if op, ok := item[method].(map[string]any); ok {
				if d, _ := op["deprecated"].(bool); d {
					deprecated = true
				}
			}
		}
		if !deprecated {
			t.Errorf("legacy path %q must be marked deprecated", p)
		}
	}
}

// collectSchemaRefs gathers every "$ref" string in a parsed JSON tree.
func collectSchemaRefs(v any, refs *[]string) {
	switch val := v.(type) {
	case map[string]any:
		if r, ok := val["$ref"].(string); ok {
			*refs = append(*refs, r)
		}
		for _, child := range val {
			collectSchemaRefs(child, refs)
		}
	case []any:
		for _, child := range val {
			collectSchemaRefs(child, refs)
		}
	}
}

// TestOpenAPISpecRefsResolve asserts every $ref in the document points at a
// schema that actually exists under components.schemas, and that the spec is
// not empty of component references.
func TestOpenAPISpecRefsResolve(t *testing.T) {
	router, _ := newTestAPIRouter()
	spec := fetchOpenAPISpec(t, router)
	components, _ := spec["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)

	var refs []string
	collectSchemaRefs(spec, &refs)
	if len(refs) == 0 {
		t.Fatal("spec contains no schema references")
	}
	for _, r := range refs {
		const prefix = "#/components/schemas/"
		if !strings.HasPrefix(r, prefix) {
			t.Errorf("unexpected $ref target %q (only component schemas are referenced)", r)
			continue
		}
		name := strings.TrimPrefix(r, prefix)
		if _, ok := schemas[name]; !ok {
			t.Errorf("$ref %q does not resolve to a component schema", r)
		}
	}
}

// ginPatternToOpenAPI converts a Gin route pattern (/api/v1/urls/:code,
// /static/*filepath) into the OpenAPI {param} form used by the spec document.
func ginPatternToOpenAPI(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		if part != "" && (part[0] == ':' || part[0] == '*') {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

// routerHasRouteFor reports whether the engine registered method + specPath
// (spec paths use {param}; the engine registers :param / *param patterns).
func routerHasRouteFor(router *gin.Engine, method, specPath string) bool {
	for _, rt := range router.Routes() {
		if rt.Method == method && ginPatternToOpenAPI(rt.Path) == specPath {
			return true
		}
	}
	return false
}

// TestOpenAPISpecMatchesRegisteredRoutes proves the spec and the router agree
// in both directions: every route the production builder registers (via
// newTestAPIRouter → newAppRouter) is described in the document with its
// method, and every spec operation resolves to a live route. This is the
// drift guard between docs/openapi.json and main.go's route assembly.
func TestOpenAPISpecMatchesRegisteredRoutes(t *testing.T) {
	router, _ := newTestAPIRouter()
	spec := fetchOpenAPISpec(t, router)
	paths, _ := spec["paths"].(map[string]any)

	// Collapse HEAD into GET: Gin serves HEAD for GET routes it registers on
	// the static file server; the spec describes the GET-shaped operation.
	registered := make(map[string]map[string]bool)
	for _, rt := range router.Routes() {
		method := rt.Method
		if method == "HEAD" {
			method = "GET"
		}
		sp := ginPatternToOpenAPI(rt.Path)
		if registered[sp] == nil {
			registered[sp] = make(map[string]bool)
		}
		registered[sp][method] = true
	}

	// Every registered route must exist in the spec (path and method).
	for sp, methods := range registered {
		item, ok := paths[sp].(map[string]any)
		if !ok {
			t.Errorf("router registers %q but OpenAPI spec has no such path", sp)
			continue
		}
		for method := range methods {
			if _, ok := item[strings.ToLower(method)]; !ok {
				t.Errorf("router registers %s %q but the OpenAPI spec has no %s operation for it", method, sp, strings.ToLower(method))
			}
		}
	}

	// Every spec operation must be a live route.
	for sp, item := range paths {
		m, ok := item.(map[string]any)
		if !ok {
			t.Errorf("path %q: expected a path item object, got %T", sp, item)
			continue
		}
		for _, method := range []string{"get", "post", "put", "delete"} {
			if _, ok := m[method]; !ok {
				continue
			}
			if !routerHasRouteFor(router, strings.ToUpper(method), sp) {
				t.Errorf("spec documents %s %q but the router does not register it", strings.ToUpper(method), sp)
			}
		}
	}
}
