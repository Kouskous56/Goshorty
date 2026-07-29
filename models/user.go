package models

const (
	// RoleAdmin is the administrator role with full access
	RoleAdmin = "admin"
	// RoleUser is the standard user role with limited access
	RoleUser = "user"
)

// User represents a user in the system
type User struct {
	ID        string `json:"id"`
	Username  string `json:"username" binding:"required,min=3,max=50"`
	Password  string `json:"-"`
	Email     string `json:"email" binding:"required,email"`
	Role      string `json:"role"`
	CreatedAt int64  `json:"created_at"`
	// TokenVersion is persisted but never exposed. Incrementing it revokes all
	// previously issued tokens for this user.
	TokenVersion int64 `json:"-"`
}

// LoginRequest is the login request
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse is the login response
type LoginResponse struct {
	Token   string `json:"token"`
	User    *User  `json:"user"`
	Message string `json:"message"`
}

// RegisterRequest is the registration request
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
}

// ChangePasswordRequest is used by an authenticated user to rotate a password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}
