# API Complete Reference

## Production operational endpoints

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| `GET` | `/health` | Public | Process liveness |
| `GET` | `/ready` | Public | Database readiness |
| `GET` | `/version` | Public | Version, commit and build timestamp |
| `GET` | `/metrics` | `Bearer METRICS_TOKEN` in release | Prometheus metrics |
| `POST` | `/api/auth/revoke` | User bearer token | Revoke every session for the current user |

`POST /api/auth/revoke` invalidates the token used for the request as well as
all other tokens previously issued to that user. The next protected request
with an old token returns HTTP 401.

## Base URL
```
http://goshorty.localhost:8080/api
```

## Authentication
All endpoints (except `/auth/login` and `/auth/register`) require:
```
Authorization: Bearer <token>
```

Where `<token>` is obtained from login or register.

## Endpoints Summary

### Authentication (No Auth Required)

| Method | Endpoint | Purpose | Auth |
|--------|----------|---------|------|
| POST | `/auth/register` | Register new user | ❌ No |
| POST | `/auth/login` | Login user | ❌ No |
| GET | `/auth/me` | Current user info | ✅ Yes |

### URL Management (Auth Required)

| Method | Endpoint | Purpose | Auth | Role |
|--------|----------|---------|------|------|
| POST | `/shorten` | Create short URL | ✅ | Any |
| GET | `/shorten/:code` | Get URL info | ✅ | Any |
| DELETE | `/shorten/:code` | Delete URL | ✅ | Any |
| GET | `/shorten/all` | List all URLs | ✅ | Any |
| GET | `/stats` | Get stats | ✅ | Any |

### User Management (Admin Only)

| Method | Endpoint | Purpose | Auth | Role |
|--------|----------|---------|------|------|
| GET | `/auth/users` | List all users | ✅ | Admin |
| PUT | `/auth/users/:username/role` | Update user role | ✅ | Admin |
| DELETE | `/auth/users/:username` | Delete user | ✅ | Admin |

---

## Detailed Endpoint Documentation

### POST `/auth/register`
Register a new user account.

**Request:**
```json
{
  "username": "john",
  "email": "john@example.com",
  "password": "securepass123"
}
```

**Validation:**
- `username`: 3-50 chars, alphanumeric + underscore
- `email`: valid email format
- `password`: minimum 6 characters

**Response (201 Created):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "abc123def456",
    "username": "john",
    "email": "john@example.com",
    "role": "user",
    "created_at": 1708123456
  },
  "message": "User registered successfully"
}
```

**Error (400 Bad Request):**
```json
{
  "message": "username already exists",
  "code": "REGISTRATION_FAILED"
}
```

---

### POST `/auth/login`
Authenticate and get token.

**Request:**
```json
{
  "username": "john",
  "password": "securepass123"
}
```

**Response (200 OK):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "abc123def456",
    "username": "john",
    "email": "john@example.com",
    "role": "user",
    "created_at": 1708123456
  },
  "message": "Login successful"
}
```

**Error (401 Unauthorized):**
```json
{
  "message": "Invalid credentials",
  "code": "INVALID_CREDENTIALS"
}
```

---

### GET `/auth/me`
Get current authenticated user info.

**Headers:**
```
Authorization: Bearer <token>
```

**Response (200 OK):**
```json
{
  "user": {
    "id": "abc123def456",
    "username": "john",
    "email": "john@example.com",
    "role": "user"
  },
  "role": "user",
  "username": "john"
}
```

---

### POST `/shorten`
Create a new short URL.

**Headers:**
```
Authorization: Bearer <token>
Content-Type: application/json
```

**Request:**
```json
{
  "url": "https://github.com/torvalds/linux",
  "expires_in": "24h",
  "custom_code": "linux"
}
```

**Parameters:**
- `url` (required): Valid URL to shorten
- `expires_in` (optional): One of: `5m`, `15m`, `1h`, `24h`, `168h` (default: `24h`)
- `custom_code` (optional): 3-50 alphanumeric chars (must be unique)

**Response (201 Created):**
```json
{
  "id": "xyz789abc123",
  "short_url": "http://goshorty.localhost:8080/s/linux",
  "short_code": "linux",
  "original_url": "https://github.com/torvalds/linux",
  "expires_in": "24h",
  "expires_at": "2026-02-17T14:30:00Z",
  "created_at": "2026-02-16T14:30:00Z"
}
```

**Error (400 Bad Request):**
```json
{
  "message": "custom code 'linux' is already taken",
  "code": "CREATION_FAILED"
}
```

---

### GET `/shorten/:code`
Get information about a shortened URL.

**Headers:**
```
Authorization: Bearer <token>
```

**URL:**
```
GET /api/shorten/linux
```

**Response (200 OK):**
```json
{
  "id": "xyz789abc123",
  "short_code": "linux",
  "original_url": "https://github.com/torvalds/linux",
  "expires_in": "24h",
  "expires_at": "2026-02-17T14:30:00Z",
  "created_at": "2026-02-16T14:30:00Z",
  "visits": 5
}
```

**Error (404 Not Found):**
```json
{
  "message": "URL not found or has expired",
  "code": "NOT_FOUND"
}
```

---

### DELETE `/shorten/:code`
Delete a shortened URL.

**Headers:**
```
Authorization: Bearer <token>
```

**URL:**
```
DELETE /api/shorten/linux
```

**Response (200 OK):**
```json
{
  "message": "URL deleted successfully",
  "code": "linux"
}
```

---

### GET `/shorten/all`
List all URLs.

**Headers:**
```
Authorization: Bearer <token>
```

**Special Notes:**
- Regular users see their own URLs
- Admins see all URLs in system

**Response (200 OK):**
```json
{
  "urls": [
    {
      "id": "xyz789abc123",
      "short_code": "linux",
      "original_url": "https://github.com/torvalds/linux",
      "expires_in": "24h",
      "expires_at": "2026-02-17T14:30:00Z",
      "created_at": "2026-02-16T14:30:00Z",
      "visits": 5
    },
    {
      "id": "abc123def456",
      "short_code": "golang",
      "original_url": "https://golang.org",
      "expires_in": "7d",
      "expires_at": "2026-02-23T14:30:00Z",
      "created_at": "2026-02-16T14:30:00Z",
      "visits": 12
    }
  ]
}
```

---

### GET `/stats`
Get system statistics.

**Headers:**
```
Authorization: Bearer <token>
```

**Response (200 OK):**
```json
{
  "total_urls": 42,
  "total_visits": 256
}
```

---

### GET `/auth/users` (Admin)
List all users in system.

**Headers:**
```
Authorization: Bearer <admin_token>
```

**Response (200 OK):**
```json
{
  "users": [
    {
      "id": "admin123",
      "username": "admin",
      "email": "admin@goshorty.local",
      "role": "admin",
      "created_at": 1708123456
    },
    {
      "id": "user456",
      "username": "john",
      "email": "john@example.com",
      "role": "user",
      "created_at": 1708123789
    }
  ]
}
```

---

### PUT `/auth/users/:username/role` (Admin)
Update a user's role (promote/demote).

**Headers:**
```
Authorization: Bearer <admin_token>
Content-Type: application/json
```

**Request:**
```json
{
  "role": "admin"
}
```

**Valid Roles:**
- `"user"` - Regular user
- `"admin"` - Administrator

**Response (200 OK):**
```json
{
  "message": "User role updated",
  "username": "john",
  "role": "admin"
}
```

---

### DELETE `/auth/users/:username` (Admin)
Delete a user from the system.

**Headers:**
```
Authorization: Bearer <admin_token>
```

**Response (200 OK):**
```json
{
  "message": "User deleted",
  "username": "john"
}
```

---

### GET `/goshorty/:timeout/:code` (Public)
Redirect to original URL. **No authentication required.**

**URL:**
```
GET /goshorty/24h/linux
```

**Response:**
- 301 Redirect to original URL
- Increments visit counter
- Returns 404 if expired or not found

**Example Flow:**
```
Request:  GET /goshorty/24h/linux
↓
Server checks if "linux" exists and not expired
↓
Increments visit count
↓
Response: 301 Moved Permanently
Location: https://github.com/torvalds/linux
↓
Browser follows redirect
```

---

## Error Codes

| Code | HTTP | Meaning |
|------|------|---------|
| `INVALID_REQUEST` | 400 | Missing or invalid parameters |
| `INVALID_CREDENTIALS` | 401 | Wrong username/password |
| `MISSING_TOKEN` | 401 | Authorization header missing |
| `INVALID_TOKEN` | 401 | Invalid or expired token |
| `ADMIN_REQUIRED` | 403 | Endpoint requires admin role |
| `NOT_FOUND` | 404 | Resource not found |
| `REGISTRATION_FAILED` | 400 | User creation failed |
| `CREATION_FAILED` | 400 | URL creation failed |
| `UPDATE_FAILED` | 400 | Update operation failed |

---

## Example Workflows

### Create and Share a URL

```bash
# 1. Login
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"john","password":"pass"}' \
  | jq -r '.token')

# 2. Create short URL
curl -X POST http://localhost:8080/api/shorten \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com","expires_in":"7d","custom_code":"github"}'

# 3. Share the short URL
echo "Share this: http://localhost:8080/goshorty/7d/github"

# 4. View stats
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/stats
```

### Admin Task: Promote User to Admin

```bash
# Login as admin
ADMIN_TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' \
  | jq -r '.token')

# Promote john to admin
curl -X PUT http://localhost:8080/api/auth/users/john/role \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"role":"admin"}'
```

---

## Rate Limiting

Currently **no rate limiting** implemented (for development).

For production, add:
- Per-IP rate limiting
- Per-user rate limiting
- Anti-bot measures

---

## Token Format

Tokens are JSON encoded (simplified JWT):

```json
{
  "user_id": "abc123def456",
  "username": "john",
  "role": "user",
  "issued_at": 1708123456,
  "expires_at": 1708209856
}
```

**Token Expiration:** 24 hours from issue date

---

## CORS Policy

All endpoints allow:
- **Origin:** `*` (any domain - development only!)
- **Methods:** GET, POST, PUT, DELETE, OPTIONS
- **Headers:** Content-Type, Authorization, Custom headers

⚠️ In production, restrict to your domain only.

---

## Status Codes

| Code | Usage |
|------|-------|
| 200 | Successful GET/PUT/DELETE |
| 201 | Successful POST (creation) |
| 400 | Bad request / validation error |
| 401 | Unauthorized / missing token |
| 403 | Forbidden / insufficient permissions |
| 404 | Resource not found |
| 500 | Server error |

---

## Best Practices

1. **Always send tokens** in Authorization header
2. **Use HTTPS** in production (not just HTTP)
3. **Never expose tokens** in logs or error messages
4. **Validate URLs** before submitting
5. **Handle token expiration** - re-login when needed
6. **Rate limit requests** to prevent abuse
7. **Monitor API usage** for suspicious activity

---

## Changelog

**v2.0.0 - February 16, 2026**
- Added complete authentication system
- Added user management endpoints
- Added admin-only endpoints
- Added role-based access control

**v1.0.0 - February 16, 2026**
- Initial release with basic API
