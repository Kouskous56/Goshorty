# GoShorty - URL Shortener with TTL

A production-ready URL shortener service built with Go, featuring automatic expiration (TTL) support with configurable time windows from 5 minutes to 7 days.

## Features

- ✅ Create short URLs with automatic TTL expiration
- ✅ Multiple TTL options: 5m, 15m, 1h, 24h, 7 days (168h)
- ✅ Custom short codes (optional)
- ✅ Visit tracking and analytics
- ✅ RESTful API
- ✅ In-memory storage with automatic cleanup
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
│   └── storage.go        # In-memory storage with TTL
├── utils/
│   └── random.go         # Utility functions
├── go.mod                # Go module file
└── README.md             # This file
```

## Installation

### Prerequisites
- Go 1.21 or higher
- Windows, macOS, or Linux

### Setup

1. Navigate to the project directory:
```bash
cd GoShorty
```

2. Install dependencies:
```bash
go mod download
```

3. Run the application:
```bash
go run main.go
```

The server will start on `http://localhost:8080`

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

- **In-Memory Storage:** URLs are stored in a thread-safe Go map
- **Automatic Cleanup:** Background goroutine removes expired entries every 30 seconds
- **TTL Enforcement:** Automatic expiration without external dependencies

### Concurrency

- Thread-safe operations using `sync.RWMutex`
- Goroutine for background cleanup
- Safe for concurrent read/write access

### Performance

- O(1) lookup time
- Minimal memory footprint
- Efficient cleanup process

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
- [ ] Database persistence (PostgreSQL)
- [ ] User authentication and API keys
- [ ] Analytics dashboard
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

Edit `config/config.go` to modify:
- Server port and host
- TTL options and defaults
- Base URL for short links

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
