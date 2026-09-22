# GoShorty - Architecture & Design Document

> **v3 note:** PostgreSQL persistence, authentication, rate limiting,
> observability, database migrations, and automated cleanup are implemented.
> Any “future” sections below describe the original prototype and are retained
> only as historical design context.

## Project Overview

GoShorty is a production-grade URL shortener service with Time-To-Live (TTL) support built with Go, Gin framework, and in-memory storage with automatic cleanup.

## Architecture

### High-Level Design

```
┌─────────────────────────────────────────────────────────────┐
│                    HTTP Clients (Browser/API)               │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    Gin Router & Middleware                  │
├─────────────────────────────────────────────────────────────┤
│  • CORS Support                                             │
│  • JSON Parsing                                             │
│  • Error Handling                                           │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    HTTP Handlers                            │
├─────────────────────────────────────────────────────────────┤
│  • CreateShortURL     (POST /api/v1/urls)                   │
│  • Redirect          (GET /goshorty/:timeout/:code)         │
│  • GetURLInfo        (GET /api/shorten/:code)               │
│  • DeleteURL         (DELETE /api/shorten/:code)            │
│  • GetStats          (GET /api/stats)                       │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    URL Service Layer                        │
├─────────────────────────────────────────────────────────────┤
│  • Business Logic                                           │
│  • URL Validation                                           │
│  • TTL Management                                           │
│  • Uniqueness Checks                                        │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    Storage Layer                            │
├─────────────────────────────────────────────────────────────┤
│  • In-Memory Map (Thread-safe)                              │
│  • TTL Enforcement                                          │
│  • Automatic Cleanup (30s interval)                         │
│  • Visit Tracking                                           │
└─────────────────────────────────────────────────────────────┘
```

## Component Details

### 1. Handlers (`handlers/handler.go`)

**Responsibility:** Parse HTTP requests, validate input, and format responses

Route registration: `/api/v1/*` is the canonical API surface; the legacy
`/api/*` set is kept as backward-compatible aliases, the canonical redirect is
`/r/:code` with `/s/:code` and `/goshorty/:timeout/:code` retained. See
`API_REFERENCE.md` for the full matrix.

**Key Methods:**
- `CreateShortURL()` - POST /api/v1/urls (legacy alias: POST /api/v1/urls)
- `Redirect()` - GET /r/:code (legacy aliases: GET /s/:code, GET /goshorty/:timeout/:code)
- `GetURLInfo()` - GET /api/v1/urls/:code (legacy alias: GET /api/shorten/:code)
- `DeleteURL()` - DELETE /api/v1/urls/:code (legacy alias: DELETE /api/shorten/:code)
- `GetAllURLs()` - GET /api/v1/urls (legacy alias: GET /api/shorten/all)
- `GetStats()` - GET /api/v1/stats (legacy alias: GET /api/stats)
- `Health()` - GET /health (alias: GET /health/live)
- `Ready()` - GET /ready (alias: GET /health/ready)

### 2. Service Layer (`services/url_service.go`)

**Responsibility:** Core business logic for URL management

**Key Methods:**
- `CreateShortURL()` - Generate short code, store with TTL
- `GetOriginalURL()` - Retrieve and increment visits
- `GetURLInfo()` - Get URL metadata
- `DeleteURL()` - Remove URL from storage
- `GetAllURLs()` - List active URLs
- `GetStats()` - Return aggregate statistics

**Import Cycle Protection:** Service depends on Storage and Config, never vice versa

### 3. Storage Layer (`storage/storage.go`)

**Responsibility:** Data persistence with TTL support

**Thread Safety:** Uses `sync.RWMutex` for concurrent access

**Key Features:**
- Lock-free reads during normal operation
- Automatic expiration checking
- Background cleanup goroutine (30-second interval)
- O(1) lookup and insertion time

**Data Structure:**
```go
type StorageEntry struct {
    Data      *models.URLData
    ExpiresAt time.Time
}
```

### 4. Models (`models/url.go`)

**Data Structures:**

```go
// Request
type ShortenRequest struct {
    URL        string  // Required, must be valid URL
    ExpiresIn  string  // Optional: 5m, 15m, 1h, 24h, 168h
    CustomCode string  // Optional: alphanumeric, max 50 chars
}

// Stored Data
type URLData struct {
    ID          string    // Unique identifier
    ShortCode   string    // The shortened code
    OriginalURL string    // Original URL
    ExpiresIn   string    // TTL option used
    ExpiresAt   time.Time // Expiration timestamp
    CreatedAt   time.Time // Creation timestamp
    Visits      int64     // Visit counter
}

// Response
type ShortenResponse struct {
    ID          string
    ShortURL    string    // Full short URL for client
    ShortCode   string    // Just the code
    OriginalURL string
    ExpiresIn   string
    ExpiresAt   time.Time
    CreatedAt   time.Time
}
```

### 5. Configuration (`config/config.go`)

**Centralized Configuration:**
```go
type Config struct {
    Server ServerConfig
    TTL    TTLConfig
}
```

**TTL Options:**
| Key | Duration | Use Case |
|-----|----------|----------|
| 5m | 5 minutes | Quick temporary links |
| 15m | 15 minutes | Short-lived sharing |
| 1h | 1 hour | Same-day links |
| 24h | 1 day | Default, general purpose |
| 168h | 7 days | Long-term references |

### 6. Frontend SPA (`static/`)

**Responsibility:** Browser-side presentation and session handling

| File | Role |
|---|---|
| `static/index.html` | Markup only — no inline `<script>` or event-handler attributes |
| `static/css/style.css` | Stylesheet extracted from the SPA |
| `static/js/app.js` | IIFE (`'use strict'`); `API_BASE = '/api/v1'`; `restoreSession()` validates the stored token on load via `GET /auth/me`; `apiFetch()` attaches the bearer header and converts any non-auth `401` into a session-expired logout; action buttons use event delegation through `data-action` attributes |

Static assets are embedded with `go:embed static/*` and mounted at `/static`
(`fs.Sub` + `router.StaticFS`); the SPA root is served at `/` by the NoRoute
fallback. Because `net/http`'s FileServer redirects any `/index.html` URL to
its parent directory (golang.org/issue/11857), the SPA index is intentionally
served only through `/`.

## Data Flow

### Creating a Short URL

```
POST /api/v1/urls
    ↓
[Handler] Parses JSON input
    ↓
[Validation] Checks URL format, TTL option validity
    ↓
[Service] Generates unique short code
    ↓
[Service] Determines expiration timestamp
    ↓
[Storage] Stores URLData with TTL
    ↓
[Response] Returns ShortenResponse with short URL
```

### Redirecting to Original URL

```
GET /goshorty/24h/abc123
    ↓
[Handler] Extracts short code from URL param
    ↓
[Service] Retrieves original URL from storage
    ↓
[Service] Increments visit counter
    ↓
[Storage] Returns original URL if not expired
    ↓
[Handler] Issues 301 redirect
    ↓
Browser follows redirect to original URL
```

## Concurrency Model

### Thread Safety Strategy

1. **RWMutex for Storage**
   - Multiple readers can access simultaneously
   - Writers get exclusive access
   - Minimal lock contention

2. **Background Cleanup Goroutine**
   - Runs every 30 seconds
   - Purges expired entries
   - Prevents unbounded memory growth
   - No blocking during cleanup

3. **Seeded Random Number Generator**
   - Each goroutine safe due to Go's math/rand implementation
   - Per-instance seeding prevents collisions

### Race Condition Prevention

- No global mutable state outside of Storage
- All shared data access through Storage interface
- Config is immutable after initialization
- Service layer is stateless (only uses dependencies)

## Performance Characteristics

### Time Complexity
| Operation | Complexity | Notes |
|-----------|-----------|-------|
| Create URL | O(n) | n = collision checks (avg 1-2) |
| Retrieve URL | O(1) | Direct map lookup |
| Delete URL | O(1) | Direct map deletion |
| List all | O(k) | k = number of active URLs |
| Cleanup | O(m) | m = total stored entries (periodic) |

### Memory Efficiency
- No external database overhead
- In-memory footprint per URL: ~500-800 bytes
- Automatic cleanup removes expired entries
- Typical capacity: 10K+ URLs in memory

## Error Handling

### Strategy
1. **Input Validation** - Check at Handler level
2. **Business Logic Errors** - Return from Service
3. **Storage Errors** - Wrapped with context
4. **User-Friendly Responses** - Standard error format

### Error Response Format
```json
{
  "message": "URL not found or has expired",
  "code": "NOT_FOUND"
}
```

## Future Scalability

### To Redis (for distributed systems)
1. Replace `storage/storage.go` with Redis client
2. Keep interfaces same - minimal Handler/Service changes
3. Redis handles TTL natively
4. Enable horizontal scaling

### To Database (for persistence)
1. Add PostgreSQL layer
2. Implement background sync
3. Add indexing on short_code
4. Keep in-memory cache for hot data

### Rate Limiting (future)
1. Add middleware with token bucket
2. Per-IP or per-API-key limits
3. Redis for distributed rate limiting

## Testing Strategy

### Unit Tests (`main_test.go`)
- `TestCreateShortURL` - Validation and creation
- `TestTTLExpiration` - Expiry behavior
- `TestVisitTracking` - Counter increment
- `TestCustomCode` - Custom code reservations
- `TestStatistics` - Data aggregation

### Manual Testing
- Use provided `test_api.ps1` (PowerShell) or `test_api.sh` (Bash)
- Test all TTL options
- Verify expiration behavior
- Check visit counters

### Load Testing (future)
- Benchmark: 10K+ concurrent requests
- Memory stability over time
- Cleanup efficiency

## Security Considerations

### Current Implementation
1. **URL Validation** - Checks valid URL format
2. **Input Sanitization** - Custom code alphanumerics only
3. **No Authentication** - Stateless, no user data

### Production Improvements
1. Add API key authentication
2. Rate limiting per IP/key
3. SQL injection prevention (if DB added)
4. XSS protection on redirects
5. CSRF tokens for POST operations
6. HTTPS enforcement

## Monitoring & Observability

### Current Metrics
- Total URLs stored
- Total visits across all URLs
- Individual URL visit counts

### Future Enhancements
1. Structured logging (JSON format)
2. Metrics export (Prometheus)
3. Distributed tracing (Jaeger)
4. Health check enhancements
5. Performance profiling

## Deployment

### Single Binary
- No external dependencies (except Go runtime)
- Easy containerization
- Simple orchestration

### Configuration
- Environment variables can replace config defaults
- No external service requirements
- Works offline (data stored in-memory)

### Scaling
1. **Vertical** - Increase machine resources
2. **Horizontal** - Move to Redis backend
3. **Hybrid** - Multiple instances with Redis

---

**Document Version:** 1.0  
**Last Updated:** February 16, 2026  
**Go Version:** 1.21+
