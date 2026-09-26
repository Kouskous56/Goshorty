package main

import (
	"net/http"
	"net/http/pprof"

	"github.com/gin-gonic/gin"
)

// pprofProfiles lists the runtime profiles served by registerPprof in addition
// to the built-in convenience endpoints (cmdline, profile, symbol, trace).
var pprofProfiles = []string{
	"allocs",
	"block",
	"goroutine",
	"heap",
	"mutex",
	"threadcreate",
}

// registerPprof mounts the Go runtime profiling surface under /debug/pprof.
// It is only wired in when PPROF_ENABLED=true (see OPERATIONS.md): these
// endpoints expose heap dumps, goroutine stacks and CPU profiles of the live
// process and must never be reachable from the public internet.
func registerPprof(router *gin.Engine) {
	group := router.Group("/debug/pprof")
	group.GET("", gin.WrapH(pprof.Index))
	group.GET("/", gin.WrapH(pprof.Index))
	group.GET("/cmdline", gin.WrapH(pprof.Cmdline))
	group.GET("/profile", gin.WrapH(pprof.Profile))
	group.GET("/symbol", gin.WrapH(pprof.Symbol))
	group.POST("/symbol", gin.WrapH(pprof.Symbol))
	group.GET("/trace", gin.WrapH(pprof.Trace))
	for _, name := range pprofProfiles {
		group.GET("/"+name, gin.WrapH(pprof.Handler(name)))
	}
}