# GoShorty v2.0 - Complete User & Admin Guide

## What's New in v2.0

✨ **Major Features Added:**
- 🔐 User Authentication (Login/Register)
- 👤 User Management System
- 💼 Admin Dashboard
- 🎨 Modern Web Interface
- 👥 Role-Based Access Control (Admin vs Client)
- 📊 User Statistics & Analytics

## Getting Started

### 1. Start the Server

```bash
cd GoShorty
go run main.go
```

Or use the compiled binary:
```bash
./goshorty.exe
```

You should see:
```
Starting GoShorty server on :8080
Open http://localhost:8080 in your browser
```

### 2. Open in Browser

Go to: **http://localhost:8080**

You'll see the login screen.

### 3. First Time Setup

Default admin account:
- **Username:** `admin`
- **Password:** `admin123`

**IMPORTANT:** Change this password in production!

## User Roles

### 👑 Admin
- Create and manage short URLs
- View ALL URLs in the system
- Manage all users
- Change user roles
- Delete users

### 👤 Regular User (Client)
- Create short URLs
- View only their own URLs
- Manage their own URLs
- View their stats

## Frontend Walkthrough

### Login Screen

```
┌─────────────────────────────────────┐
│         🔓 Login                    │
│                                     │
│ Username: [__________________]      │
│ Password: [__________________]      │
│                                     │
│        [Login Button]               │
│                                     │
│ Don't have account? Register here   │
└─────────────────────────────────────┘
```

**Features:**
- Toggle between Login and Register
- Error messages for invalid credentials
- LocalStorage remembers token
- Auto-redirect to dashboard if logged in

### Client Dashboard

#### Tab 1: ✂️ Shorten URL

Create new short URLs:

```
Original URL: [https://example.com/very/long/url]
Expiration:   [24h ▼]
Custom Code:  [optional_code]

[Create Short URL]

Result:
┌──────────────────────────────┐
│ Code: abc123                 │ [Copy]
│ Short URL: http://...        │
│ Expires: Feb 17, 2026        │
└──────────────────────────────┘
```

**TTL Options:**
- 5 minutes - Quick temporary links
- 15 minutes - Short-term sharing
- 1 hour - Same-day content
- 24 hours - Default, general use
- 7 days - Long-term references

#### Tab 2: 📋 My URLs

View all your shortened URLs:

```
Stats:
┌──────────────┬──────────────┐
│ 5 Total URLs │ 42 Visits    │
└──────────────┴──────────────┘

URLs:
┌──────────────────────────────────┐
│ Code: abc123     [Open] [Copy]   │
│ Original: https://example.com    │
│ Visits: 5,  Expires: Feb 17     │
│ [Delete]                         │
└──────────────────────────────────┘
```

**Actions:**
- **Open** - Test the redirect
- **Copy** - Copy original URL
- **Delete** - Remove the short URL

### Admin Dashboard

#### Tab 3: 👁️ All URLs (Admin Only)

View every URL in the system:

```
Stats:
┌──────────────┬──────────────┐
│ 42 Total URLs│ 256 Visits   │
└──────────────┴──────────────┘

Lists all URLs with delete option
```

#### Tab 4: 👥 Users (Admin Only)

Manage all users:

```
┌─────────────┬──────────────┬────────┬────────────┬──────────┐
│ Username    │ Email        │ Role   │ Created    │ Actions  │
├─────────────┼──────────────┼────────┼────────────┼──────────┤
│ admin       │ admin@...    │ ADMIN  │ Feb 16     │ Change   │
│ john        │ john@...     │ USER   │ Feb 16     │ To Admin │
│ jane        │ jane@...     │ USER   │ Feb 16     │ To Admin │
│             │              │        │            │ Delete   │
└─────────────┴──────────────┴────────┴────────────┴──────────┘
```

**Actions:**
- **Change Role** - Toggle between User and Admin
- **Delete** - Remove user from system

## API Endpoints Reference

### Authentication Endpoints

#### Register New User
```bash
POST /api/auth/register
Content-Type: application/json

{
  "username": "newuser",
  "email": "user@example.com",
  "password": "securepassword"
}

# Response:
{
  "token": "eyJ...",
  "user": {
    "id": "abc123",
    "username": "newuser",
    "email": "user@example.com",
    "role": "user",
    "created_at": 1708123456
  },
  "message": "User registered successfully"
}
```

#### Login
```bash
POST /api/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123"
}

# Response:
{
  "token": "eyJ...",
  "user": { ... },
  "message": "Login successful"
}
```

#### Get Current User Info
```bash
GET /api/auth/me
Authorization: Bearer <token>

# Response:
{
  "user": { ... },
  "role": "admin",
  "username": "admin"
}
```

### URL Endpoints (Protected)

#### Create Short URL
```bash
POST /api/shorten
Authorization: Bearer <token>
Content-Type: application/json

{
  "url": "https://example.com/very/long/url",
  "expires_in": "24h",
  "custom_code": "mycode"  # optional
}

# Response:
{
  "id": "xyz789",
  "short_url": "http://localhost:8080/goshorty/24h/mycode",
  "short_code": "mycode",
  "original_url": "https://example.com/very/long/url",
  "expires_in": "24h",
  "expires_at": "2026-02-17T12:00:00Z",
  "created_at": "2026-02-16T12:00:00Z"
}
```

#### Get URL Info
```bash
GET /api/shorten/:code
Authorization: Bearer <token>

# Response:
{
  "id": "xyz789",
  "short_code": "mycode",
  "original_url": "https://example.com/very/long/url",
  "expires_in": "24h",
  "expires_at": "2026-02-17T12:00:00Z",
  "created_at": "2026-02-16T12:00:00Z",
  "visits": 5
}
```

#### Delete URL
```bash
DELETE /api/shorten/:code
Authorization: Bearer <token>

# Response:
{
  "message": "URL deleted successfully",
  "code": "mycode"
}
```

#### List All URLs
```bash
GET /api/shorten/all
Authorization: Bearer <token>

# Response:
{
  "urls": [ ... ]
}
```

#### Get Statistics
```bash
GET /api/stats
Authorization: Bearer <token>

# Response:
{
  "total_urls": 42,
  "total_visits": 256
}
```

### Admin Endpoints (Protected & Admin Only)

#### List All Users
```bash
GET /api/auth/users
Authorization: Bearer <admin_token>

# Response:
{
  "users": [
    {
      "id": "abc123",
      "username": "admin",
      "email": "admin@...",
      "role": "admin",
      "created_at": 1708123456
    },
    ...
  ]
}
```

#### Update User Role
```bash
PUT /api/auth/users/:username/role
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "role": "admin"  # or "user"
}

# Response:
{
  "message": "User role updated",
  "username": "john",
  "role": "admin"
}
```

#### Delete User
```bash
DELETE /api/auth/users/:username
Authorization: Bearer <admin_token>

# Response:
{
  "message": "User deleted",
  "username": "john"
}
```

### Redirect Endpoint (No Auth Required)

#### Redirect to Original URL
```bash
GET /goshorty/:timeout/:code

# Example:
GET /goshorty/24h/mycode
# → Redirects to original URL
```

## Using cURL Examples

### Register
```bash
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "john",
    "email": "john@example.com",
    "password": "password123"
  }'
```

### Login and Save Token
```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "john",
    "password": "password123"
  }' | grep -o '"token":"[^"]*' | cut -d'"' -f4)

echo "Token: $TOKEN"
```

### Create Short URL (with token)
```bash
curl -X POST http://localhost:8080/api/shorten \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "url": "https://github.com",
    "expires_in": "24h",
    "custom_code": "github"
  }'
```

### Get My URLs
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/shorten/all
```

### Admin: List Users
```bash
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/api/auth/users
```

### Admin: Delete User
```bash
curl -X DELETE \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/api/auth/users/john
```

## Security Features

✅ **Password Hashing**
- Passwords hashed with SHA256
- Never stored in plain text

✅ **Token-Based Authentication**
- JWT-like tokens
- 24-hour expiration
- Sent in Authorization header

✅ **Role-Based Access**
- Admin-only endpoints protected
- Users can only access their own data

✅ **Input Validation**
- Email format validation
- URL format validation
- Custom code alphanumeric only

## Troubleshooting

### Frontend Not Loading
1. Make sure server is running: `go run main.go`
2. Check browser console (F12) for errors
3. Verify `static/index.html` exists
4. Clear browser cache: Ctrl+Shift+Delete

### Login Failed
- Check username and password
- Default: `admin` / `admin123`
- Verify user exists (ask admin to create)

### API Errors
- Check token in browser DevTools (F12 → Application → LocalStorage)
- Verify Authorization header is sent
- Check server logs for detailed errors

### CORS Issues
- The server has CORS enabled for all origins
- This is set for development; restrict in production

### Expired Session
- Tokens expire after 24 hours
- Login again to get a new token
- Token is auto-removed from browser on logout

## Production Checklist

📋 **Before deploying to production:**

- [ ] Change default admin password
- [ ] Use HTTPS instead of HTTP
- [ ] Set proper CORS origins (not *)
- [ ] Use environment variables for secrets
- [ ] Enable database persistence (MongoDB/PostgreSQL)
- [ ] Add rate limiting
- [ ] Set up monitoring/logging
- [ ] Use proper JWT library (not simplified)
- [ ] Add email verification for registration
- [ ] Enable password reset functionality
- [ ] Add audit logging
- [ ] Regular database backups

## File Structure

```
GoShorty/
├── static/
│   └── index.html          ← Modern web UI
├── models/
│   ├── url.go
│   └── user.go            ← NEW: User models
├── services/
│   ├── url_service.go
│   └── token_service.go   ← NEW: Auth token handling
├── handlers/
│   ├── handler.go
│   └── auth.go           ← NEW: Auth endpoints
├── storage/
│   ├── storage.go
│   └── user.go           ← NEW: User storage
└── main.go               ← Updated: Auth routes
```

## Next Steps

1. ✅ Register a new user
2. ✅ Create your first short URL
3. ✅ Test the redirect
4. ✅ (Admin) Create another user
5. ✅ (Admin) View all URLs
6. ✅ (Admin) Manage users
7. ✅ Deploy to hosting service

## Version History

**v2.0.0** (February 16, 2026)
- ✨ Added full authentication system
- ✨ Added user management
- ✨ Added admin dashboard
- ✨ Added modern web UI
- ✨ Added role-based access control

**v1.0.0** (February 16, 2026)
- Initial release with API only

## Support & Feedback

For issues or suggestions:
- Check console logs: `F12` → Console tab
- Review error messages in UI
- Check API response in Network tab (F12)

---

**GoShorty v2.0** - Complete URL Shortener Solution  
Built with Go, Gin, and vanilla JavaScript
