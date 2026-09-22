# API Complete Reference

## API versioning

`/api/v1/*` is the canonical API surface. The legacy `/api/*` routes remain
fully functional as deprecated aliases for backward compatibility and are
labelled *legacy* below. New integrations should target `/api/v1`.

The canonical short-link redirect is `/r/:code` (the format returned on
creation). `/s/:code` and `/goshorty/:timeout/:code` continue to work so
previously issued links never break.

## Production operational endpoints

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| `GET` | `/health` | Public | Process liveness |
| `GET` | `/health/live` | Public | Process liveness (canonical alias) |
| `GET` | `/ready` | Public | Database readiness |
| `GET` | `/health/ready` | Public | Database readiness (canonical alias) |
| `GET` | `/version` | Public | Version, commit and build timestamp |
| `GET` | `/metrics` | `Bearer METRICS_TOKEN` in release | Prometheus metrics |
| `POST` | `/api/v1/auth/revoke` | User bearer token | Revoke every session for the current user |

`POST /api/v1/auth/revoke` invalidates the token used for the request as well as
all other tokens previously issued to that user. The next protected request
with an old token returns HTTP 401. The legacy `POST /api/auth/revoke` alias
behaves identically.

## Base URL
```
http://goshorty.localhost:8080/api/v1
```

The legacy API lives under `http://goshorty.localhost:8080/api`.

## Authentication
All endpoints (except `/auth/login` and `/auth/register`) require:
```
Authorization: Bearer <token>
```

Where `<token>` is obtained from login or register.

## Endpoints Summary

Paths below are canonical `/api/v1/...`; each also works at its legacy alias
(shown in parentheses) with identical behavior.

### Authentication (No Auth Required)

| Method | Endpoint (legacy alias) | Purpose | Auth |
|--------|----------|---------|------|
| POST | `/api/v1/auth/register` (`/api/auth/register`) | Register new user | ❌ No |
| POST | `/api/v1/auth/login` (`/api/auth/login`) | Login user | ❌ No |
| GET | `/api/v1/auth/me` (`/api/auth/me`) | Current user info | ✅ Yes |

### URL Management (Auth Required)

| Method | Endpoint (legacy alias) | Purpose | Auth | Role |
|--------|----------|---------|------|------|
| POST | `/api/v1/urls` (`/api/shorten`) | Create short URL | ✅ | Any |
| GET | `/api/v1/urls/:code` (`/api/shorten/:code`) | Get URL info | ✅ | Any |
| DELETE | `/api/v1/urls/:code` (`/api/shorten/:code`) | Delete URL | ✅ | Any |
| GET | `/api/v1/urls` (`/api/shorten/all`) | List all URLs | ✅ | Any |
| GET | `/api/v1/stats` (`/api/stats`) | Get stats | ✅ | Any |

### User Management (Admin Only)

| Method | Endpoint (legacy alias) | Purpose | Auth | Role |
|--------|----------|---------|------|------|
| GET | `/api/v1/auth/users` (`/api/auth/users`) | List all users | ✅ | Admin |
| PUT | `/api/v1/auth/users/:username/role` (`/api/auth/users/:username/role`) | Update user role | ✅ | Admin |
| DELETE | `/api/v1/auth/users/:username` (`/api/auth/users/:username`) | Delete user | ✅ | Admin |

### Redirect (Public)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/r/:code` | Canonical redirect to original URL |
| GET | `/s/:code` | Legacy redirect alias |
| GET | `/goshorty/:timeout/:code` | Legacy redirect route (backward compatible) |

---

## Detailed Endpoint Documentation

Paths and request/response shapes below are identical on `/api/v1` and on the
legacy `/api` aliases.

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
  "short_url": "http://goshorty.localhost:8080/r/linux",
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

### GET `/r/:code` (Public)
Canonical redirect to original URL. **No authentication required.**

**URL:**
```
GET /r/linux
```

**Backward-compatible aliases:**
- `GET /s/linux`
- `GET /goshorty/24h/linux` (legacy format)

**Response:**
- 302 Redirect to original URL
- Increments visit counter
- Returns 404 if expired or not found

**Example Flow:**
```
Request:  GET /r/linux
↓
Server checks if "linux" exists and not expired
↓
Increments visit count
↓
Response: 302 Found
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

# 2. Create short URL (canonical v1 endpoint)
curl -X POST http://localhost:8080/api/v1/urls \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com","expires_in":"7d","custom_code":"github"}'

# 3. Share the short URL (canonical redirect format)
echo "Share this: http://localhost:8080/r/github"

# 4. View stats
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/stats
```

### Admin Task: Promote User to Admin

```bash
# Login as admin
ADMIN_TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' \
  | jq -r '.token')

# Promote john to admin
curl -X PUT http://localhost:8080/api/v1/auth/users/john/role \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"role":"admin"}'
```

---

## Rate Limiting

In-memory per-key rate limiting is applied on sensitive endpoints: login,
register, password change, session revocation, URL creation and redirects.
Limits are defined in `main.go` (for example login 10/min, register 5/hour,
shorten 60/min, redirect 300/min).

---

## Token Format

Tokens use the self-defined `base64url(payload).base64url(signature)` format
signed with HMAC-SHA256. The payload contains:

```json
{
  "jti": "ab12cd34ef56",
  "user_id": "abc123def456",
  "username": "john",
  "role": "user",
  "token_version": 0,
  "iss": "",
  "aud": "",
  "kid": "v1",
  "issued_at": 1708123456,
  "expires_at": 1708209856
}
```

**Token Expiration:** `TOKEN_TTL` (default 24 hours) from issue date. See
`SECURITY.md` ("Token format and hardening") for verification rules and key
rotation.

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

**v2.1.0 - September 22, 2026**
- Canonical `/api/v1/*` API surface; legacy `/api/*` kept as backward-compatible aliases
- Canonical redirect `/r/:code`; `/s/:code` and `/goshorty/:timeout/:code` retained
- `/health/live` and `/health/ready` canonical health aliases (`/health`, `/ready` unchanged)

**v2.0.0 - February 16, 2026**
- Added complete authentication system
- Added user management endpoints
- Added admin-only endpoints
- Added role-based access control

**v1.0.0 - February 16, 2026**
- Initial release with basic API
