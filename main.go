package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"goshorty/config"
	"goshorty/handlers"
	"goshorty/services"
	"goshorty/storage"
)

//go:embed static/*
var staticFiles embed.FS

var (
	version   = "3.0.0"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
	log.SetFlags(0)
	log.SetOutput(slog.NewLogLogger(logger.Handler(), slog.LevelInfo).Writer())

	// Load configuration
	cfg := config.NewConfig()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Validate required environment variables
	secretKey := os.Getenv("SECRET_KEY")
	if secretKey == "" {
		log.Fatal("SECRET_KEY environment variable is required")
	}
	if gin.Mode() == gin.ReleaseMode && len([]byte(secretKey)) < 32 {
		log.Fatal("SECRET_KEY must be at least 32 bytes in release mode")
	}
	// SECRET_KEY_PREVIOUS enables smooth key rotation: tokens signed with the
	// previous key keep verifying until it is removed (see OPERATIONS.md).
	previousSecretKey := os.Getenv("SECRET_KEY_PREVIOUS")
	if previousSecretKey != "" {
		if gin.Mode() == gin.ReleaseMode && len([]byte(previousSecretKey)) < 32 {
			log.Fatal("SECRET_KEY_PREVIOUS must be at least 32 bytes in release mode")
		}
		if previousSecretKey == secretKey {
			log.Fatal("SECRET_KEY_PREVIOUS must differ from SECRET_KEY")
		}
	}

	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		if gin.Mode() == gin.ReleaseMode {
			log.Fatal("ADMIN_PASSWORD environment variable is required in release mode")
		}
		adminPassword = "admin123"
		log.Println("WARNING: using insecure default ADMIN_PASSWORD for development only")
	}
	if gin.Mode() == gin.ReleaseMode && (len([]byte(adminPassword)) < 12 || len([]byte(adminPassword)) > 72) {
		log.Fatal("ADMIN_PASSWORD must be between 12 and 72 bytes in release mode")
	}
	adminEmail := getEnv("ADMIN_EMAIL", "admin@goshorty.local")

	// Initialize persistence. Production requires PostgreSQL; local development
	// may use the in-memory implementation for a zero-setup workflow.
	var urlStorage storage.URLStore
	var userStorage storage.UserStore
	var healthChecker storage.HealthChecker
	var closeStorage func()

	if cfg.Database.URL != "" {
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
		postgresStore, err := storage.NewPostgresStore(dbCtx, cfg.Database.URL, adminPassword, adminEmail)
		dbCancel()
		if err != nil {
			log.Fatalf("Failed to initialize PostgreSQL storage: %v", err)
		}

		cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
		go postgresStore.RunCleanup(cleanupCtx, time.Minute, 500)

		urlStorage = postgresStore
		userStorage = postgresStore
		healthChecker = postgresStore
		closeStorage = func() {
			cleanupCancel()
			postgresStore.Close()
		}
		log.Println("Using PostgreSQL persistent storage")
	} else {
		if gin.Mode() == gin.ReleaseMode {
			log.Fatal("DATABASE_URL environment variable is required in release mode")
		}

		memoryURLs := storage.NewStorage()
		memoryUsers, err := storage.NewUserStorage(adminPassword, adminEmail)
		if err != nil {
			log.Fatalf("Failed to initialize in-memory user storage: %v", err)
		}
		urlStorage = memoryURLs
		userStorage = memoryUsers
		closeStorage = memoryURLs.Stop
		log.Println("WARNING: using non-persistent in-memory storage for development only")
	}

	// Initialize services
	urlService := services.NewURLService(urlStorage, cfg)
	tokenService := services.NewTokenService(secretKey,
		services.WithTTL(cfg.Security.TokenTTL),
		services.WithPreviousSecret(previousSecretKey),
		services.WithIssuer(cfg.Security.TokenIssuer),
		services.WithAudience(cfg.Security.TokenAudience),
	)

	// Initialize handlers
	h := handlers.NewHandlerWithHealth(urlService, healthChecker)
	authHandler := handlers.NewAuthHandler(userStorage, tokenService)

	// Create Gin router
	router := gin.New()
	if err := router.SetTrustedProxies(cfg.Security.TrustedProxies); err != nil {
		log.Fatalf("Invalid TRUSTED_PROXIES configuration: %v", err)
	}

	// Middleware
	metrics := NewHTTPMetrics()
	router.Use(observabilityMiddleware(logger, metrics))
	router.Use(structuredRecovery(logger))
	router.Use(securityHeadersMiddleware())
	router.Use(corsMiddleware(cfg.Security.AllowedOrigins))
	router.Use(requestBodyLimitMiddleware(cfg.Security.MaxRequestBytes))

	// Serve embedded static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("Failed to load embedded static files: %v", err)
	}
	router.StaticFS("/static", http.FS(staticFS))

	// Health and readiness. /health/live and /health/ready are canonical
	// aliases; /health and /ready remain as backward-compatible endpoints.
	router.GET("/health", h.Health)
	router.GET("/health/live", h.Health)
	router.GET("/ready", h.Ready)
	router.GET("/health/ready", h.Ready)
	router.GET("/metrics", metricsAuthMiddleware(cfg.Security.MetricsToken), metrics.Handler)
	router.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"version":    version,
			"commit":     commit,
			"build_time": buildTime,
		})
	})

	rateLimiter := handlers.NewRateLimiter()

	// Canonical API v1 and legacy /api aliases. Both share the same handlers;
	// only the URL-management paths differ (/urls on v1, /shorten on legacy).
	// The v1 surface adds pagination for list endpoints; legacy keeps the
	// original response shapes.
	registerAPIGroup(router.Group("/api"), authHandler, h, rateLimiter, "/shorten", "/shorten/all", "")
	registerAPIGroup(router.Group("/api/v1"), authHandler, h, rateLimiter, "/urls", "/urls", "v1")

	// Canonical compact redirect route. Expiration is authoritative in storage,
	// so it does not need to be encoded into the public URL.
	router.GET("/r/:code", rateLimiter.Limit("redirect", 300, time.Minute), h.Redirect)

	// Backward-compatible redirect aliases so previously issued links keep
	// working (/s/:code and the legacy /goshorty/:timeout/:code format).
	router.GET("/s/:code", rateLimiter.Limit("redirect", 300, time.Minute), h.Redirect)
	router.GET("/goshorty/:timeout/:code", rateLimiter.Limit("redirect", 300, time.Minute), h.Redirect)

	// API info endpoints.
	registerAPIInfo(router, version)

	// Serve index.html for all other routes (SPA fallback)
	router.NoRoute(func(c *gin.Context) {
		// For API requests, return 404
		if len(c.Request.URL.Path) > 4 && c.Request.URL.Path[:4] == "/api" {
			c.JSON(404, gin.H{"error": "Not found"})
			return
		}
		// For other routes, serve index.html (SPA)
		c.Header("Content-Type", "text/html; charset=utf-8")
		indexData, _ := fs.ReadFile(staticFiles, "static/index.html")
		c.String(200, string(indexData))
	})

	// Start server
	addr := cfg.Server.Port
	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Channel to listen for OS signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	serverErrors := make(chan error, 1)

	// Start server in goroutine
	go func() {
		log.Printf("Starting GoShorty server on %s", addr)
		log.Printf("Open %s in your browser", cfg.Server.BaseURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// Wait for a shutdown signal or an unexpected listener failure.
	select {
	case sig := <-quit:
		log.Printf("Received signal: %v, shutting down...", sig)
	case err := <-serverErrors:
		closeStorage()
		log.Fatalf("HTTP server failed: %v", err)
	}

	// Graceful shutdown with 5s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		closeStorage()
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	closeStorage()
	log.Println("Server exited cleanly")
}

// registerAPIInfo mounts the descriptive /api and /api/v1 endpoints.
func registerAPIInfo(router *gin.Engine, version string) {
	router.GET("/api", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service":       "GoShorty - URL Shortener with TTL",
			"version":       version,
			"frontend":      "/",
			"canonical_api": "/api/v1",
			"endpoints": gin.H{
				"GET /health":                        "Health check",
				"GET /health/live":                   "Health check alias",
				"GET /ready":                         "PostgreSQL readiness check",
				"GET /health/ready":                  "Readiness check alias",
				"GET /metrics":                       "Prometheus-compatible HTTP metrics",
				"GET /version":                       "Release build metadata",
				"GET /r/:code":                       "Redirect to original URL (canonical)",
				"GET /s/:code":                       "Legacy redirect alias",
				"GET /goshorty/:timeout/:code":       "Legacy redirect route",
				"POST /api/auth/login":               "Login user (legacy alias)",
				"POST /api/auth/register":            "Register new user (legacy alias)",
				"GET /api/auth/me":                   "Current user info (legacy alias)",
				"PUT /api/auth/password":             "Change current user password (legacy alias)",
				"POST /api/auth/revoke":              "Revoke all current-user sessions (legacy alias)",
				"POST /api/shorten":                  "Create short URL (legacy alias)",
				"GET /api/shorten/:code":             "Get URL info (legacy alias)",
				"DELETE /api/shorten/:code":          "Delete URL (legacy alias)",
				"GET /api/shorten/all":               "List all URLs (legacy alias)",
				"GET /api/stats":                     "Get stats (legacy alias)",
				"GET /api/auth/users":                "List users (admin, legacy alias)",
				"PUT /api/auth/users/:username/role": "Update user role (admin, legacy alias)",
				"DELETE /api/auth/users/:username":   "Delete user (admin, legacy alias)",
			},
		})
	})
	router.GET("/api/v1", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service":  "GoShorty - URL Shortener with TTL",
			"version":  version,
			"frontend": "/",
			"endpoints": gin.H{
				"GET /health":                           "Health check",
				"GET /health/live":                      "Health check alias",
				"GET /ready":                            "PostgreSQL readiness check",
				"GET /health/ready":                     "Readiness check alias",
				"GET /metrics":                          "Prometheus-compatible HTTP metrics",
				"GET /version":                          "Release build metadata",
				"GET /r/:code":                          "Redirect to original URL (canonical)",
				"POST /api/v1/auth/login":               "Login user",
				"POST /api/v1/auth/register":            "Register new user",
				"GET /api/v1/auth/me":                   "Current user info",
				"PUT /api/v1/auth/password":             "Change current user password",
				"POST /api/v1/auth/revoke":              "Revoke all current-user sessions",
				"POST /api/v1/urls":                     "Create short URL",
				"GET /api/v1/urls":                      "List URLs (paginated, newest first)",
				"GET /api/v1/urls/:code":                "Get URL info",
				"DELETE /api/v1/urls/:code":             "Delete URL",
				"GET /api/v1/stats":                     "Get stats",
				"GET /api/v1/auth/users":                "List users (admin, paginated)",
				"PUT /api/v1/auth/users/:username/role": "Update user role (admin)",
				"DELETE /api/v1/auth/users/:username":   "Delete user (admin)",
			},
		})
	})
}

// registerAPIGroup wires the auth and URL routes into a router group.
// urlPath is the URL-management base and listPath its collection route
// ("/urls" and "/urls" on v1; "/shorten" and "/shorten/all" on the legacy
// alias). Both register the same handlers, so the /api/v1 surface is purely
// additive and all legacy /api routes keep working unchanged.
func registerAPIGroup(rg *gin.RouterGroup, authHandler *handlers.AuthHandler, h *handlers.Handler, rateLimiter *handlers.RateLimiter, urlPath, listPath, apiVersion string) {
	// Tag every request with the API surface so list handlers can decide
	// between the paginated canonical behavior (v1) and the legacy shape.
	rg.Use(func(c *gin.Context) {
		c.Set("api_version", apiVersion)
		c.Next()
	})
	// Auth routes (no auth required)
	auth := rg.Group("/auth")
	{
		auth.POST("/login", rateLimiter.Limit("login", 10, time.Minute), authHandler.Login)
		auth.POST("/register", rateLimiter.Limit("register", 5, time.Hour), authHandler.Register)
	}

	// Protected routes (auth required)
	protected := rg.Group("")
	protected.Use(authHandler.AuthMiddleware())
	{
		// Current user info
		protected.GET("/auth/me", authHandler.GetCurrentUser)
		protected.PUT("/auth/password", rateLimiter.Limit("password", 5, time.Hour), authHandler.ChangePassword)
		protected.POST("/auth/revoke", rateLimiter.Limit("revoke", 5, time.Hour), authHandler.RevokeSessions)

		// URL management
		protected.POST(urlPath, rateLimiter.Limit("shorten", 60, time.Minute), h.CreateShortURL)
		protected.GET(urlPath+"/:code", h.GetURLInfo)
		protected.DELETE(urlPath+"/:code", h.DeleteURL)
		protected.GET(listPath, h.GetAllURLs)
		protected.GET("/stats", h.GetStats)

		// Admin routes
		admin := protected.Group("")
		admin.Use(handlers.AdminMiddleware())
		{
			admin.GET("/auth/users", authHandler.GetAllUsers)
			admin.PUT("/auth/users/:username/role", authHandler.UpdateUserRole)
			admin.DELETE("/auth/users/:username", authHandler.DeleteUser)
		}
	}
}

// getEnv returns environment variable value or fallback default
func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
