package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"goshorty/models"
	"goshorty/services"
)

// Handler holds all HTTP handlers and their dependencies
type Handler struct {
	urlService *services.URLService
}

// NewHandler creates a new handler instance
func NewHandler(urlService *services.URLService) *Handler {
	return &Handler{
		urlService: urlService,
	}
}

// CreateShortURL handles POST /api/shorten
func (h *Handler) CreateShortURL(c *gin.Context) {
	var req models.ShortenRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
		return
	}

	userID := c.GetString("user_id")

	// Create short URL
	response, err := h.urlService.CreateShortURL(&req, userID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrCodeTaken):
			writeError(c, http.StatusConflict, "SHORT_CODE_TAKEN", "Short code is already taken")
		case errors.Is(err, services.ErrInvalidURL):
			writeError(c, http.StatusBadRequest, "INVALID_URL", err.Error())
		default:
			writeError(c, http.StatusInternalServerError, "CREATION_FAILED", "Failed to create short URL")
		}
		return
	}

	c.JSON(http.StatusCreated, response)
}

// requireCode checks if a short code is present in the URL params.
// Returns false and sends a 400 error if the code is empty.
func requireCode(c *gin.Context, code string) bool {
	if code == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Short code is required",
			Code:    "MISSING_CODE",
		})
		return false
	}
	return true
}

// Redirect handles GET /goshorty/:timeout/:code
func (h *Handler) Redirect(c *gin.Context) {
	code := c.Param("code")

	if !requireCode(c, code) {
		return
	}

	originalURL, err := h.urlService.GetOriginalURL(code)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Message: "URL not found or has expired",
			Code:    "NOT_FOUND",
		})
		return
	}

	c.Redirect(http.StatusFound, originalURL)
}

// GetURLInfo handles GET /api/shorten/:code
func (h *Handler) GetURLInfo(c *gin.Context) {
	code := c.Param("code")

	if !requireCode(c, code) {
		return
	}

	userID := c.GetString("user_id")
	role := c.GetString("role")
	urlData, err := h.urlService.GetURLInfo(code, userID, role)
	if err != nil {
		writeError(c, http.StatusNotFound, "NOT_FOUND", "URL not found or has expired")
		return
	}

	c.JSON(http.StatusOK, urlData)
}

// DeleteURL handles DELETE /api/shorten/:code
func (h *Handler) DeleteURL(c *gin.Context) {
	code := c.Param("code")

	if !requireCode(c, code) {
		return
	}

	userID := c.GetString("user_id")
	role := c.GetString("role")

	err := h.urlService.DeleteURL(code, userID, role)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			writeError(c, http.StatusNotFound, "NOT_FOUND", "URL not found or has expired")
			return
		}
		if errors.Is(err, services.ErrURLNotFound) {
			writeError(c, http.StatusNotFound, "NOT_FOUND", "URL not found or has expired")
			return
		}
		writeError(c, http.StatusInternalServerError, "DELETE_FAILED", "Failed to delete URL")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "URL deleted successfully",
		"code":    code,
	})
}

// GetAllURLs handles GET /api/shorten/all
func (h *Handler) GetAllURLs(c *gin.Context) {
	userID := c.GetString("user_id")
	role := c.GetString("role")
	urls, err := h.urlService.GetAllURLs(userID, role)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "LIST_FAILED", "Failed to list URLs")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"urls": urls,
	})
}

// GetStats handles GET /api/stats
func (h *Handler) GetStats(c *gin.Context) {
	userID := c.GetString("user_id")
	role := c.GetString("role")
	stats, err := h.urlService.GetStats(userID, role)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "STATS_FAILED", "Failed to load statistics")
		return
	}
	c.JSON(http.StatusOK, stats)
}

// Health handles GET /health
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "goshorty",
	})
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, models.ErrorResponse{
		Message: message,
		Code:    code,
	})
}
