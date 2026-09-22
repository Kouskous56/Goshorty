# GoShorty - URL Shortener with TTL

Current release: **v3.0.0**. The canonical local address is
`http://goshorty.localhost:8080`; public deployments use the hostname supplied
through `PUBLIC_BASE_URL`.

A production-ready URL shortener service built with Go, featuring automatic expiration (TTL) support with configurable time windows from 5 minutes to 7 days.

## Features

Production hardening includes origin-allowlisted CORS, browser security headers,
per-route rate limiting, a 1 MiB request-body limit, authenticated password
rotation, and separate liveness/PostgreSQL readiness checks.

- ✅ Create short URLs with automatic TTL expiration
- ✅ Multiple TTL options: 5m, 15m, 1h, 24h, 7 days (168h)
- ✅ Custom short codes (optional)
- ✅ Visit tracking and analytics
- ✅ RESTful API
- ✅ PostgreSQL persistence in production
- ✅ In-memory storage fallback for local development
- ✅ CORS enabled
- ✅ Production-ready error handling

## Project Structure

```
GoShorty/
├── main.go                 # Application entry point
├── config/
│   └── config.go          # Configuration management
├── models/
│   └── url.go            # Data models
├── services/
│   └── url_service.go    # Business logic
├── handlers/
│   └── handler.go        # HTTP request handlers
├── storage/
│   ├── interfaces.go     # Repository contracts
│   ├── storage.go        # In-memory development storage
│   ├── postgres.go       # PostgreSQL production storage
│   └── migrations/       # Embedded database migrations
├── utils/
│   └── random.go         # Utility functions
├── go.mod                # Go module file
└── README.md             # This file
```

## Installation

### Prerequisites
- Go 1.26.5 or higher (includes required standard-library security fixes)
- Windows, macOS, or Linux

### Setup

Recommended environment bootstrap:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/setup-env.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/doctor.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev.ps1
```

On Linux/macOS/WSL:

```bash
bash scripts/setup-env.sh
bash scripts/doctor.sh
bash scripts/dev.sh
```

This starts PostgreSQL 16 through Docker Compose and runs GoShorty with the
ignored `.env.local`. See [ENVIRONMENTS.md](ENVIRONMENTS.md) for development,
test, CI, and Railway production configuration.

1. Navigate to the project directory:
```bash
cd GoShorty
```

2. Install dependencies:
```bash
go mod download
```

3. Set a development signing key and run the application:
```bash
# PowerShell
$env:SECRET_KEY="development-secret-change-me"
go run .

# Linux/macOS
SECRET_KEY="development-secret-change-me" go run .
```

The server will start on `http://goshorty.localhost:8080` when using the
generated local environment. `http://localhost:8080` remains usable for direct
access.

## Quality gate

GitHub Actions runs formatting, module verification, vet, unit tests, coverage
thresholds, a Linux race detector, `govulncheck`, PostgreSQL integration tests,
and an end-to-end API flow for every pull request and push to `main`.

Local equivalents:

```bash
go test ./... -count=1
go vet ./...
go test ./... -covermode=atomic -coverprofile=coverage.out
bash scripts/check-coverage.sh coverage.out 30
```

See [INTEGRATION_TESTING.md](INTEGRATION_TESTING.md) for PostgreSQL, race, and
E2E instructions.

## API Documentation

### Create Short URL

**Endpoint:** `POST /api/shorten`

**Request Body:**
```json
{
  "url": "https://example.com/very/long/url/that/needs/shortening",
  "expires_in": "24h",
  "custom_code": "mycode"
}
```

**Parameters:**
- `url` (required): The original URL to shorten
- `expires_in` (optional): TTL - one of: `5m`, `15m`, `1h`, `24h`, `168h` (default: `24h`)
- `custom_code` (optional): Custom short code (must be alphanumeric, max 50 chars)

**Response:**
```json
{
  "id": "a1b2c3d4e5f6g7h8",
  "short_url": "http://localhost:8080/goshorty/24h/mycode",
  "short_code": "mycode",
  "original_url": "https://example.com/very/long/url",
  "expires_in": "24h",
  "expires_at": "2026-02-17T12:00:00Z",
  "created_at": "2026-02-16T12:00:00Z"
}
```

### Redirect to Original URL

**Endpoint:** `GET /goshorty/:timeout/:code`

Example: `GET /goshorty/24h/mycode`

Redirects to the original URL if not expired. Returns 404 if expired or not found.

### Get URL Information

**Endpoint:** `GET /api/shorten/:code`

**Response:**
```json
{
  "id": "a1b2c3d4e5f6g7h8",
  "short_code": "mycode",
  "original_url": "https://example.com/very/long/url",
  "expires_in": "24h",
  "expires_at": "2026-02-17T12:00:00Z",
  "created_at": "2026-02-16T12:00:00Z",
  "visits": 5
}
```

### Delete Short URL

**Endpoint:** `DELETE /api/shorten/:code`

**Response:**
```json
{
  "message": "URL deleted successfully",
  "code": "mycode"
}
```

### Get All URLs

**Endpoint:** `GET /api/shorten/all`

**Response:**
```json
{
  "urls": [...]
}
```

### Get Statistics

**Endpoint:** `GET /api/stats`

**Response:**
```json
{
  "total_urls": 42,
  "total_visits": 156
}
```

### Health Check

**Endpoint:** `GET /health`

**Response:**
```json
{
  "status": "ok",
  "service": "goshorty"
}
```

### Readiness Check

**Endpoint:** `GET /ready`

Returns `200` only when required production dependencies, including PostgreSQL,
are reachable. Railway uses this endpoint for deployment health checks.

**Endpoint:** `GET /metrics`

Returns Prometheus-compatible HTTP request counters and duration summaries.
Every response also carries `X-Request-ID`, which matches the structured JSON
request log. Release deployments require
`Authorization: Bearer <METRICS_TOKEN>`.

**Endpoint:** `GET /version`

Returns the application version, source commit, and build timestamp.

Operational procedures, backup/restore drills, alerts, and deployment
verification are documented in [OPERATIONS.md](OPERATIONS.md). Security
boundaries and token revocation semantics are documented in
[SECURITY.md](SECURITY.md).

### Change Password

**Endpoint:** `PUT /api/auth/password`

Requires `Authorization: Bearer <token>`.

```json
{
  "current_password": "current-password",
  "new_password": "new-unique-password"
}
```

The new password must be between 12 and 72 bytes.

## Usage Examples

### Using cURL

**Create a short URL:**
```bash
curl -X POST http://localhost:8080/api/shorten \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://www.google.com",
    "expires_in": "1h",
    "custom_code": "google"
  }'
```

**Get URL info:**
```bash
curl http://localhost:8080/api/shorten/google
```

**Delete short URL:**
```bash
curl -X DELETE http://localhost:8080/api/shorten/google
```

### Using JavaScript/Fetch

```javascript
// Create short URL
const response = await fetch('http://localhost:8080/api/shorten', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    url: 'https://www.example.com',
    expires_in: '24h'
  })
});

const data = await response.json();
console.log(data.short_url);
```

## TTL Options

| Option | Duration | Use Case |
|--------|----------|----------|
| 5m | 5 minutes | Quick temporary links |
| 15m | 15 minutes | Short-lived sharing |
| 1h | 1 hour | Same-day links |
| 24h | 1 day | Default option |
| 168h | 7 days | Long-term sharing |

## Technical Details

### Storage

- **Production:** PostgreSQL selected through `DATABASE_URL`
- **Development:** Thread-safe in-memory fallback when not in release mode
- **Migrations:** Embedded, versioned SQL migrations run automatically at startup
- **Consistency:** Unique short-code constraint, transactional admin invariants, atomic visit counters
- **Cleanup:** Expired URLs are removed in bounded background batches

### Production security

- Login: 10 requests/minute/IP
- Registration: 5 requests/hour/IP
- Password changes: 5 requests/hour/IP
- URL creation: 60 requests/minute/IP
- Redirects: 300 requests/minute/IP
- Request bodies: maximum 1 MiB
- CORS defaults to `PUBLIC_BASE_URL`; override with comma-separated
  `ALLOWED_ORIGINS`
- Trusted reverse proxies default to Railway's `100.64.0.0/10`; override with
  comma-separated `TRUSTED_PROXIES`
- HTTP timeouts: 5s headers, 15s read/write, 60s idle

### Concurrency

- Thread-safe operations using `sync.RWMutex`
- Goroutine for background cleanup
- Safe for concurrent read/write access

### Performance

- Indexed short-code and expiry lookups
- PostgreSQL connection pooling through pgx
- Horizontal instances share the same persistent data

## Architecture

```
Request → Router → Handler → Service → Storage
                     ↑                    ↓
                  Config ← ← ← ← ← ← ← ← 
```

1. **Handlers**: Accept HTTP requests, validate input
2. **Services**: Implement business logic, coordinate operations
3. **Storage**: Manage data persistence and TTL
4. **Config**: Centralized configuration management

## Future Enhancements

- [ ] Redis backend support for distributed deployments
- [x] Database persistence (PostgreSQL)
- [x] User authentication and role-based access
- [x] Analytics dashboard
- [ ] Batch URL shortening
- [ ] QR code generation
- [ ] URL preview feature
- [ ] Advanced rate limiting
- [ ] Webhook support
- [ ] Docker containerization

## Development

### Build

```bash
go build -o goshorty.exe
```

### Run Binary

```bash
./goshorty.exe
```

### Testing

```bash
go test ./...
```

## Configuration

Production configuration is provided through environment variables:

- `SECRET_KEY` — required token signing key.
- `ADMIN_PASSWORD` — required in release mode.
- `DATABASE_URL` — required in release mode.
- `PUBLIC_BASE_URL` — canonical public origin for short links.
- `ADMIN_EMAIL` — optional bootstrap admin email.
- `TOKEN_TTL` — optional token lifetime, default `24h`.
- `PORT` — listening port; Railway supplies this automatically.
- `GIN_MODE=release` — enables production mode.

## Error Handling

The API returns appropriate HTTP status codes:
- `200 OK`: Successful request
- `201 Created`: URL successfully created
- `400 Bad Request`: Invalid input
- `404 Not Found`: URL not found or expired
- `500 Internal Server Error`: Server error

## License

MIT License - Feel free to use this project

## Support

For issues or questions, please open an issue in the repository.

---

**GoShorty v1.0.0** - Built with Go & Gin Framework
