package main

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

// OpenAPI 3.1 specification for the canonical API surface, embedded at build
// time and served from GET /api/v1/openapi.json.
//
//go:embed docs/openapi.json
var openAPISpec []byte

// registerOpenAPI serves the embedded OpenAPI 3.1 document. The spec covers
// the canonical /api/v1 surface, the operational endpoints, the public
// redirect routes and the legacy /api aliases (marked deprecated). It is a
// static document: no Swagger UI, no runtime-generated schemas and no extra
// dependencies.
func registerOpenAPI(router *gin.Engine) {
	router.GET("/api/v1/openapi.json", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json; charset=utf-8", openAPISpec)
	})
}
