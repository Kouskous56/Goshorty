package handlers

import (
	"net/http"
	"strings"
	"goshorty/models"
	"goshorty/services"
	"goshorty/storage"
	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication
type AuthHandler struct {
	userStorage    *storage.UserStorage
	tokenService   *services.TokenService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(userStorage *storage.UserStorage, tokenService *services.TokenService) *AuthHandler {
	return &AuthHandler{
		userStorage:  userStorage,
		tokenService: tokenService,
	}
}

// Register handles user registration
func (ah *AuthHandler) Register(c *gin.Context) {
	var req models.RegisterRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid request: " + err.Error(),
			Code:    "INVALID_REQUEST",
		})
		return
	}

	user, err := ah.userStorage.CreateUser(req.Username, req.Password, req.Email)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Registration failed",
			Code:    "REGISTRATION_FAILED",
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
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid request: " + err.Error(),
			Code:    "INVALID_REQUEST",
		})
		return
	}

	// Verify password
	valid, err := ah.userStorage.VerifyPassword(req.Username, req.Password)
	if err != nil || !valid {
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
	users := ah.userStorage.GetAllUsers()
	c.JSON(http.StatusOK, gin.H{
		"users": users,
	})
}

// UpdateUserRole updates a user's role (admin only)
func (ah *AuthHandler) UpdateUserRole(c *gin.Context) {
	username := c.Param("username")
	var req map[string]string

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid request: " + err.Error(),
			Code:    "INVALID_REQUEST",
		})
		return
	}

	role := req["role"]
	if err := ah.userStorage.UpdateUserRole(username, role); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Failed to update user role",
			Code:    "UPDATE_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User role updated",
		"username": username,
		"role": role,
	})
}

// DeleteUser deletes a user (admin only)
func (ah *AuthHandler) DeleteUser(c *gin.Context) {
	username := c.Param("username")

	if err := ah.userStorage.DeleteUser(username); err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Message: "User not found",
			Code:    "NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User deleted",
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

		// Store user info in context
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

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
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Message: "User not found",
			Code:    "USER_NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
		"role": role,
		"username": username,
	})
}
