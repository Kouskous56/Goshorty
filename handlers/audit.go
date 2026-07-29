package handlers

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

// auditSecurityEvent emits a bounded structured event without request bodies,
// credentials, tokens, or authorization headers.
func auditSecurityEvent(c *gin.Context, event, outcome string, attributes ...any) {
	fields := []any{
		"event", event,
		"outcome", outcome,
		"request_id", c.GetString("request_id"),
		"actor_user_id", c.GetString("user_id"),
		"actor_username", c.GetString("username"),
		"client_ip", c.ClientIP(),
	}
	fields = append(fields, attributes...)
	slog.Info("security_audit", fields...)
}
