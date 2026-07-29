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
	tokenTTL := getEnv("TOKEN_TTL", "24h")
	ttl, err := time.ParseDuration(tokenTTL)
	if err == nil {
		services.TokenTTL = ttl
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
	tokenService := services.NewTokenService(secretKey)

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

	// Health check (no auth required)
	router.GET("/health", h.Health)
	router.GET("/ready", h.Ready)
	router.GET("/metrics", metricsAuthMiddleware(cfg.Security.MetricsToken), metrics.Handler)
	router.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"version":    version,
			"commit":     commit,
			"build_time": buildTime,
		})
	})

	rateLimiter := handlers.NewRateLimiter()

	// API v1 routes
	api := router.Group("/api")
	{
		// Auth routes (no auth required)
		auth := api.Group("/auth")
		{
			auth.POST("/login", rateLimiter.Limit("login", 10, time.Minute), authHandler.Login)
			auth.POST("/register", rateLimiter.Limit("register", 5, time.Hour), authHandler.Register)
		}

		// Protected routes (auth required)
		protected := api.Group("")
		protected.Use(authHandler.AuthMiddleware())
		{
			// Current user info
			protected.GET("/auth/me", authHandler.GetCurrentUser)
			protected.PUT("/auth/password", rateLimiter.Limit("password", 5, time.Hour), authHandler.ChangePassword)
			protected.POST("/auth/revoke", rateLimiter.Limit("revoke", 5, time.Hour), authHandler.RevokeSessions)

			// URL shortening (protected)
			protected.POST("/shorten", rateLimiter.Limit("shorten", 60, time.Minute), h.CreateShortURL)
			protected.GET("/shorten/:code", h.GetURLInfo)
			protected.DELETE("/shorten/:code", h.DeleteURL)
			protected.GET("/shorten/all", h.GetAllURLs)
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

	// Redirect route - handles goshorty/[timeout]/[code]
	router.GET("/goshorty/:timeout/:code", rateLimiter.Limit("redirect", 300, time.Minute), h.Redirect)

	// Root route (API info)
	router.GET("/api", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"service": "GoShorty - URL Shortener with TTL",
			"version": version,
			"features": []string{
				"URL shortening with auto-expiration",
				"User authentication",
				"Admin dashboard",
				"User management",
			},
			"frontend": "/",
			"endpoints": gin.H{
				"GET /health":                        "Health check",
				"GET /ready":                         "PostgreSQL readiness check",
				"GET /metrics":                       "Prometheus-compatible HTTP metrics",
				"GET /version":                       "Release build metadata",
				"GET /goshorty/:timeout/:code":       "Redirect to original URL",
				"POST /api/auth/login":               "Login user",
				"POST /api/auth/register":            "Register new user",
				"GET /api/auth/me":                   "Current user info",
				"PUT /api/auth/password":             "Change current user password",
				"POST /api/auth/revoke":              "Revoke all current-user sessions",
				"POST /api/shorten":                  "Create short URL",
				"GET /api/shorten/:code":             "Get URL info",
				"DELETE /api/shorten/:code":          "Delete URL",
				"GET /api/shorten/all":               "List all URLs",
				"GET /api/stats":                     "Get stats",
				"GET /api/auth/users":                "List users (admin)",
				"PUT /api/auth/users/:username/role": "Update user role (admin)",
				"DELETE /api/auth/users/:username":   "Delete user (admin)",
			},
		})
	})

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

// getEnv returns environment variable value or fallback default
func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
