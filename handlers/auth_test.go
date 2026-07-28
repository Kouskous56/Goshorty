package handlers

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"goshorty/models"
	"goshorty/services"
	"goshorty/storage"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupAuthTest() (*gin.Engine, *storage.UserStorage, *services.TokenService) {
	gin.SetMode(gin.TestMode)
	userStorage, err := storage.NewUserStorage("test-admin", "admin@goshorty.local")
	if err != nil {
		panic(err)
	}
	tokenService := services.NewTokenService("test-secret-key")
	authHandler := NewAuthHandler(userStorage, tokenService)

	router := gin.New()
	auth := router.Group("/api/auth")
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/register", authHandler.Register)
		auth.GET("/me", authHandler.AuthMiddleware(), authHandler.GetCurrentUser)
	}

	protected := router.Group("/api")
	protected.Use(authHandler.AuthMiddleware())
	{
		protected.GET("/auth/users", AdminMiddleware(), authHandler.GetAllUsers)
		protected.PUT("/auth/users/:username/role", AdminMiddleware(), authHandler.UpdateUserRole)
		protected.DELETE("/auth/users/:username", AdminMiddleware(), authHandler.DeleteUser)
	}

	return router, userStorage, tokenService
}

func tokenFor(t *testing.T, ts *services.TokenService, userID, username, role string) string {
	token, err := ts.GenerateToken(userID, username, role)
	if err != nil {
		t.Fatalf("Failed to generate test token: %v", err)
	}
	return token
}

func TestRegister_Success(t *testing.T) {
	router, _, _ := setupAuthTest()

	body, _ := json.Marshal(models.RegisterRequest{
		Username: "newuser",
		Password: "password123",
		Email:    "new@test.com",
	})
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected 201 Created, got %d", w.Code)
	}

	var resp models.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal("Failed to parse response body")
	}

	if resp.Token == "" {
		t.Error("Expected non-empty token")
	}
	if resp.User == nil {
		t.Fatal("Expected user in response")
	}
	if resp.User.Username != "newuser" {
		t.Errorf("Expected username 'newuser', got '%s'", resp.User.Username)
	}
	if resp.User.Role != models.RoleUser {
		t.Errorf("Expected role '%s', got '%s'", models.RoleUser, resp.User.Role)
	}
	if resp.User.Password != "" {
		t.Error("Password should never be returned in JSON")
	}
}

func TestRegister_InvalidInput(t *testing.T) {
	router, _, _ := setupAuthTest()

	tests := []struct {
		name    string
		payload map[string]string
	}{
		{"Empty username", map[string]string{"username": "", "password": "pass123", "email": "a@b.com"}},
		{"Short password", map[string]string{"username": "user1", "password": "123", "email": "a@b.com"}},
		{"Bad email", map[string]string{"username": "user1", "password": "password123", "email": "notanemail"}},
		{"Missing fields", map[string]string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected 400, got %d", w.Code)
			}
		})
	}
}

func TestRegister_Duplicate(t *testing.T) {
	router, _, _ := setupAuthTest()

	payload := models.RegisterRequest{
		Username: "dupuser",
		Password: "password123",
		Email:    "dup@test.com",
	}
	body, _ := json.Marshal(payload)

	req1, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("First registration should succeed, got %d", w1.Code)
	}

	req2, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Errorf("Duplicate registration should return 409, got %d", w2.Code)
	}
}

func TestLogin_Success(t *testing.T) {
	router, _, _ := setupAuthTest()

	body, _ := json.Marshal(models.LoginRequest{
		Username: "admin",
		Password: "test-admin",
	})
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", w.Code)
	}

	var resp models.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal("Failed to parse response body")
	}

	if resp.Token == "" {
		t.Error("Expected non-empty token")
	}
	if resp.User == nil {
		t.Fatal("Expected user in response")
	}
	if resp.User.Username != "admin" {
		t.Errorf("Expected username 'admin', got '%s'", resp.User.Username)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	router, _, _ := setupAuthTest()

	body, _ := json.Marshal(models.LoginRequest{
		Username: "admin",
		Password: "wrong-password",
	})
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestLogin_NonexistentUser(t *testing.T) {
	router, _, _ := setupAuthTest()

	body, _ := json.Marshal(models.LoginRequest{
		Username: "nonexistent",
		Password: "somepass",
	})
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_NoHeader(t *testing.T) {
	router, _, _ := setupAuthTest()

	req, _ := http.NewRequest("GET", "/api/auth/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_WrongFormat(t *testing.T) {
	router, _, _ := setupAuthTest()

	req, _ := http.NewRequest("GET", "/api/auth/me", nil)
	req.Header.Set("Authorization", "InvalidFormat")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	router, _, _ := setupAuthTest()

	req, _ := http.NewRequest("GET", "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-here")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	router, us, ts := setupAuthTest()
	newUser, _ := us.CreateUser("authtest", "password123", "auth@test.com")
	token := tokenFor(t, ts, newUser.ID, "authtest", models.RoleUser)

	req, _ := http.NewRequest("GET", "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAdminMiddleware_Forbidden(t *testing.T) {
	router, us, ts := setupAuthTest()
	user, _ := us.CreateUser("regular", "password123", "regular@test.com")
	token := tokenFor(t, ts, user.ID, user.Username, models.RoleUser)

	req, _ := http.NewRequest("GET", "/api/auth/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403, got %d", w.Code)
	}
}

func TestAdminMiddleware_Allowed(t *testing.T) {
	router, us, ts := setupAuthTest()
	adminUser, _ := us.GetUser("admin")
	token := tokenFor(t, ts, adminUser.ID, "admin", models.RoleAdmin)

	req, _ := http.NewRequest("GET", "/api/auth/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetAllUsers(t *testing.T) {
	router, us, ts := setupAuthTest()
	adminUser, _ := us.GetUser("admin")
	token := tokenFor(t, ts, adminUser.ID, "admin", models.RoleAdmin)

	req, _ := http.NewRequest("GET", "/api/auth/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}

	var resp map[string][]*models.User
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal("Failed to parse response")
	}

	if len(resp["users"]) < 1 {
		t.Error("Expected at least 1 user")
	}
}

func TestUpdateUserRole(t *testing.T) {
	router, us, ts := setupAuthTest()

	us.CreateUser("target", "pass123", "target@test.com")

	adminUser, _ := us.GetUser("admin")
	token := tokenFor(t, ts, adminUser.ID, "admin", models.RoleAdmin)

	body, _ := json.Marshal(map[string]string{"role": models.RoleAdmin})
	req, _ := http.NewRequest("PUT", "/api/auth/users/target/role", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	updatedUser, _ := us.GetUser("target")
	if updatedUser.Role != models.RoleAdmin {
		t.Errorf("Expected role '%s', got '%s'", models.RoleAdmin, updatedUser.Role)
	}
}

func TestUpdateUserRole_InvalidRole(t *testing.T) {
	router, us, ts := setupAuthTest()

	us.CreateUser("target2", "pass123", "target2@test.com")

	adminUser, _ := us.GetUser("admin")
	token := tokenFor(t, ts, adminUser.ID, "admin", models.RoleAdmin)

	body, _ := json.Marshal(map[string]string{"role": "superadmin"})
	req, _ := http.NewRequest("PUT", "/api/auth/users/target2/role", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestDeleteUser(t *testing.T) {
	router, us, ts := setupAuthTest()

	us.CreateUser("delete-me", "pass123", "delete@test.com")

	adminUser, _ := us.GetUser("admin")
	token := tokenFor(t, ts, adminUser.ID, "admin", models.RoleAdmin)

	req, _ := http.NewRequest("DELETE", "/api/auth/users/delete-me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	_, err := us.GetUser("delete-me")
	if err == nil {
		t.Error("Expected user to be deleted")
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	router, us, ts := setupAuthTest()

	adminUser, _ := us.GetUser("admin")
	token := tokenFor(t, ts, adminUser.ID, "admin", models.RoleAdmin)

	req, _ := http.NewRequest("DELETE", "/api/auth/users/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestAuthMiddlewareRejectsDeletedUserToken(t *testing.T) {
	router, us, ts := setupAuthTest()
	user, _ := us.CreateUser("deleted-session", "password123", "deleted@test.com")
	token := tokenFor(t, ts, user.ID, user.Username, user.Role)
	if err := us.DeleteUser(user.Username); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest("GET", "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected deleted user's token to return 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareUsesCurrentRole(t *testing.T) {
	router, us, ts := setupAuthTest()
	user, _ := us.CreateUser("role-session", "password123", "role@test.com")
	if err := us.UpdateUserRole(user.Username, models.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	token := tokenFor(t, ts, user.ID, user.Username, models.RoleAdmin)
	if err := us.UpdateUserRole(user.Username, models.RoleUser); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest("GET", "/api/auth/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected stale admin token to return 403, got %d", w.Code)
	}
}

func TestCannotRemoveLastAdminThroughAPI(t *testing.T) {
	router, us, ts := setupAuthTest()
	admin, _ := us.GetUser("admin")
	token := tokenFor(t, ts, admin.ID, admin.Username, admin.Role)

	body, _ := json.Marshal(map[string]string{"role": models.RoleUser})
	roleReq, _ := http.NewRequest("PUT", "/api/auth/users/admin/role", bytes.NewBuffer(body))
	roleReq.Header.Set("Content-Type", "application/json")
	roleReq.Header.Set("Authorization", "Bearer "+token)
	roleW := httptest.NewRecorder()
	router.ServeHTTP(roleW, roleReq)
	if roleW.Code != http.StatusConflict {
		t.Fatalf("expected final admin demotion to return 409, got %d", roleW.Code)
	}

	deleteReq, _ := http.NewRequest("DELETE", "/api/auth/users/admin", nil)
	deleteReq.Header.Set("Authorization", "Bearer "+token)
	deleteW := httptest.NewRecorder()
	router.ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusConflict {
		t.Fatalf("expected final admin deletion to return 409, got %d", deleteW.Code)
	}
}
