package handlers

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"goshorty/models"
)

type rateWindow struct {
	start time.Time
	count int
}

// RateLimiter is a bounded, in-process fixed-window limiter.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateWindow
	now     func() time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{entries: make(map[string]rateWindow), now: time.Now}
}

// Limit rejects requests above limit within window. keyScope separates routes.
func (l *RateLimiter) Limit(keyScope string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		now := l.now()
		key := keyScope + ":" + c.ClientIP()

		l.mu.Lock()
		entry := l.entries[key]
		if entry.start.IsZero() || now.Sub(entry.start) >= window {
			entry = rateWindow{start: now}
		}
		entry.count++
		l.entries[key] = entry
		remaining := limit - entry.count
		if remaining < 0 {
			remaining = 0
		}
		retryAfter := int(window.Seconds() - now.Sub(entry.start).Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}
		// Opportunistic cleanup keeps attacker-controlled keys bounded over time.
		if len(l.entries) > 10_000 {
			for storedKey, stored := range l.entries {
				if now.Sub(stored.start) >= window {
					delete(l.entries, storedKey)
				}
			}
			for storedKey := range l.entries {
				if len(l.entries) <= 10_000 {
					break
				}
				delete(l.entries, storedKey)
			}
		}
		exceeded := entry.count > limit
		l.mu.Unlock()

		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if exceeded {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, models.ErrorResponse{
				Message: "Too many requests, please try again later",
				Code:    "RATE_LIMITED",
			})
			return
		}
		c.Next()
	}
}
