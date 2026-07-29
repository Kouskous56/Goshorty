package handlers

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityAuditEventExcludesAuthorizationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("POST", "/api/auth/revoke", nil)
	context.Request.Header.Set("Authorization", "Bearer must-not-appear")
	context.Set("request_id", "request-1234")
	context.Set("user_id", "user-1")
	context.Set("username", "alice")
	auditSecurityEvent(context, "user.sessions_revoke", "success")

	logged := output.String()
	if !strings.Contains(logged, `"msg":"security_audit"`) ||
		!strings.Contains(logged, `"event":"user.sessions_revoke"`) {
		t.Fatalf("missing audit fields: %s", logged)
	}
	if strings.Contains(logged, "must-not-appear") {
		t.Fatalf("authorization header leaked into audit log: %s", logged)
	}
}
