package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"goshorty/models"
	"goshorty/services"
	"goshorty/storage"
)

// AuthHandler handles authentication
type AuthHandler struct {
	userStorage  storage.UserStore
	tokenService *services.TokenService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(userStorage storage.UserStore, tokenService *services.TokenService) *AuthHandler {
	return &AuthHandler{
		userStorage:  userStorage,
		tokenService: tokenService,
	}
}

// Register handles user registration
func (ah *AuthHandler) Register(c *gin.Context) {
	var req models.RegisterRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
		return
	}

	user, err := ah.userStorage.CreateUser(req.Username, req.Password, req.Email)
	if err != nil {
		if errors.Is(err, storage.ErrUsernameExists) {
			writeError(c, http.StatusConflict, "USERNAME_TAKEN", "Username is already registered")
			return
		}
		writeError(c, http.StatusInternalServerError, "REGISTRATION_FAILED", "Registration failed")
		return
	}

	token, err := ah.tokenService.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to generate token",
			Code:    "TOKEN_ERROR",
		})
		return
	}

	c.JSON(http.StatusCreated, models.LoginResponse{
		Token:   token,
		User:    user,
		Message: "User registered successfully",
	})
}

// Login handles user login
func (ah *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
		return
	}

	// Verify password
	valid, err := ah.userStorage.VerifyPassword(req.Username, req.Password)
	if err != nil {
		if !errors.Is(err, storage.ErrUserNotFound) {
			writeError(c, http.StatusInternalServerError, "AUTH_FAILED", "Authentication service unavailable")
			return
		}
		writeError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid credentials")
		return
	}
	if !valid {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "Invalid credentials",
			Code:    "INVALID_CREDENTIALS",
		})
		return
	}

	user, err := ah.userStorage.GetUser(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Internal server error",
			Code:    "INTERNAL_ERROR",
		})
		return
	}

	token, err := ah.tokenService.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to generate token",
			Code:    "TOKEN_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, models.LoginResponse{
		Token:   token,
		User:    user,
		Message: "Login successful",
	})
}

// GetAllUsers returns all users (admin only)
func (ah *AuthHandler) GetAllUsers(c *gin.Context) {
	users, err := ah.userStorage.GetAllUsers()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "LIST_FAILED", "Failed to list users")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"users": users,
	})
}

// UpdateUserRole updates a user's role (admin only)
func (ah *AuthHandler) UpdateUserRole(c *gin.Context) {
	username := c.Param("username")
	var req map[string]string

	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
		return
	}

	role := req["role"]
	if err := ah.userStorage.UpdateUserRole(username, role); err != nil {
		switch {
		case errors.Is(err, storage.ErrUserNotFound):
			writeError(c, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
		case errors.Is(err, storage.ErrInvalidRole):
			writeError(c, http.StatusBadRequest, "INVALID_ROLE", "Role must be admin or user")
		case errors.Is(err, storage.ErrLastAdmin):
			writeError(c, http.StatusConflict, "LAST_ADMIN", "The last admin cannot be demoted")
		default:
			writeError(c, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update user role")
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "User role updated",
		"username": username,
		"role":     role,
	})
}

// DeleteUser deletes a user (admin only)
func (ah *AuthHandler) DeleteUser(c *gin.Context) {
	username := c.Param("username")

	if err := ah.userStorage.DeleteUser(username); err != nil {
		switch {
		case errors.Is(err, storage.ErrUserNotFound):
			writeError(c, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
		case errors.Is(err, storage.ErrLastAdmin):
			writeError(c, http.StatusConflict, "LAST_ADMIN", "The last admin cannot be deleted")
		default:
			writeError(c, http.StatusInternalServerError, "DELETE_FAILED", "Failed to delete user")
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "User deleted",
		"username": username,
	})
}

// AuthMiddleware checks if user is authenticated
func (ah *AuthHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Message: "Missing authorization header",
				Code:    "MISSING_TOKEN",
			})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Message: "Invalid authorization format",
				Code:    "INVALID_TOKEN_FORMAT",
			})
			c.Abort()
			return
		}

		token := parts[1]

		// Verify token
		claims, err := ah.tokenService.VerifyToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Message: "Invalid or expired token",
				Code:    "INVALID_TOKEN",
			})
			c.Abort()
			return
		}

		// Resolve the current user on every request so deletion and role changes
		// take effect immediately instead of waiting for the token to expire.
		user, err := ah.userStorage.GetUserByID(claims.UserID)
		if err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				writeError(c, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid or expired token")
			} else {
				writeError(c, http.StatusInternalServerError, "AUTH_FAILED", "Authentication service unavailable")
			}
			c.Abort()
			return
		}

		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("role", user.Role)

		c.Next()
	}
}

// AdminMiddleware checks if user is admin
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != models.RoleAdmin {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Message: "Admin access required",
				Code:    "ADMIN_REQUIRED",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// GetCurrentUser returns info about the current authenticated user
func (ah *AuthHandler) GetCurrentUser(c *gin.Context) {
	username, _ := c.Get("username")
	role, _ := c.Get("role")

	user, err := ah.userStorage.GetUserByID(c.GetString("user_id"))
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			writeError(c, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
		} else {
			writeError(c, http.StatusInternalServerError, "USER_LOOKUP_FAILED", "Failed to load current user")
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":     user,
		"role":     role,
		"username": username,
	})
}
