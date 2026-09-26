# GoShorty - Quick Start Guide

## 30-Second Setup

### 1. Install Go Dependencies
```bash
cd GoShorty
go mod download
```

### 2. Run the Server
```bash
go run main.go
```

You should see:
```
Starting GoShorty server on :8080
Base URL: http://goshorty.localhost:8080
```

### Persistent Local Database (No Docker)

Without `DATABASE_URL` the app uses in-memory storage, so data resets on
restart. To run a real, persistent PostgreSQL without Docker, start the embedded
server (downloads the Postgres binary once on first use):

```bash
go run ./cmd/localdb
```

Then, in a second terminal, point the app at it:

```bash
# PowerShell
$env:DATABASE_URL="postgres://goshorty:goshorty@127.0.0.1:5433/goshorty?sslmode=disable"
go run .

# Linux/macOS
DATABASE_URL="postgres://goshorty:goshorty@127.0.0.1:5433/goshorty?sslmode=disable" go run .
```

### 3. Create a Short URL

Using curl:
```bash
curl -X POST http://localhost:8080/api/shorten \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com","expires_in":"24h"}'
```

Response:
```json
{
  "id": "abc123...",
  "short_url": "http://localhost:8080/goshorty/24h/xYz9kL",
  "short_code": "xYz9kL",
  "original_url": "https://example.com",
  "expires_in": "24h",
  "expires_at": "2026-02-17T...",
  "created_at": "2026-02-16T..."
}
```

### 4. Use Your Short URL

Visit in browser:
```
http://localhost:8080/goshorty/24h/xYz9kL
```

It will redirect to `https://example.com`

## API Endpoints

| Method | Endpoint | Purpose |
|--------|----------|---------|
| POST | `/api/shorten` | Create short URL |
| GET | `/api/shorten/:code` | Get URL info |
| GET | `/goshorty/:timeout/:code` | Redirect to URL |
| DELETE | `/api/shorten/:code` | Delete URL |
| GET | `/api/stats` | Get statistics |
| GET | `/health` | Health check |

## Example: Create with Custom Code

```bash
curl -X POST http://localhost:8080/api/shorten \
  -H "Content-Type: application/json" \
  -d '{
    "url":"https://github.com",
    "expires_in":"1h",
    "custom_code":"gh"
  }'
```

Then access: `http://localhost:8080/goshorty/1h/gh`

## TTL Options

- `5m` - 5 minutes (quick sharing)
- `15m` - 15 minutes
- `1h` - 1 hour
- `24h` - 1 day (default)
- `168h` - 7 days (long-term)

## Run Tests

```bash
go test -v ./...
```

## Build Executable

```bash
go build -o goshorty.exe
./goshorty.exe
```

## Troubleshooting

**Port 8080 already in use?**
Edit `config/config.go`, change `Port: ":8080"` to another port like `:3000`

**Module errors?**
```bash
go mod tidy
go mod download
```

---

✅ You're all set! Start creating short URLs.
