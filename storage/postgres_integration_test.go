package storage

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"goshorty/models"
	"goshorty/utils"
)

func TestPostgresPersistence(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testURL := isolatedPostgresURL(t, ctx, databaseURL)

	store, err := NewPostgresStore(ctx, testURL, "integration-admin-password", "admin@test.local")
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser("persistent-user", "password123", "user@test.local")
	if err != nil {
		t.Fatal(err)
	}
	data := &models.URLData{
		ID:          utils.GenerateID(),
		ShortCode:   "persistent-code",
		OriginalURL: "https://example.com/persistent",
		ExpiresIn:   "24h",
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		CreatedAt:   time.Now(),
		CreatedBy:   user.ID,
	}
	if err := store.SetIfAbsent(data.ShortCode, data); err != nil {
		t.Fatal(err)
	}
	store.Close()

	reopened, err := NewPostgresStore(ctx, testURL, "integration-admin-password", "admin@test.local")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	if _, err := reopened.GetUser("persistent-user"); err != nil {
		t.Fatalf("user did not persist across reconnect: %v", err)
	}
	got, err := reopened.Get("persistent-code")
	if err != nil {
		t.Fatalf("URL did not persist across reconnect: %v", err)
	}
	if got.OriginalURL != data.OriginalURL {
		t.Fatalf("unexpected persisted URL: %+v", got)
	}
	valid, err := reopened.VerifyPassword("persistent-user", "password123")
	if err != nil || !valid {
		t.Fatalf("persisted password verification failed: valid=%v err=%v", valid, err)
	}
	if err := reopened.UpdatePassword(user.ID, "replacement-password"); err != nil {
		t.Fatalf("update persisted password: %v", err)
	}
	updatedUser, err := reopened.GetUserByID(user.ID)
	if err != nil || updatedUser.TokenVersion != user.TokenVersion+1 {
		t.Fatalf("password change did not persist token revocation: user=%+v err=%v", updatedUser, err)
	}
	if err := reopened.RevokeTokens(user.ID); err != nil {
		t.Fatalf("revoke persisted sessions: %v", err)
	}
	revokedUser, err := reopened.GetUserByID(user.ID)
	if err != nil || revokedUser.TokenVersion != updatedUser.TokenVersion+1 {
		t.Fatalf("session revoke did not persist token version: user=%+v err=%v", revokedUser, err)
	}
	valid, err = reopened.VerifyPassword("persistent-user", "replacement-password")
	if err != nil || !valid {
		t.Fatalf("updated password verification failed: valid=%v err=%v", valid, err)
	}
	if err := reopened.Ping(ctx); err != nil {
		t.Fatalf("readiness ping failed: %v", err)
	}
	visited, err := reopened.GetAndIncrement("persistent-code")
	if err != nil || visited.Visits != 1 {
		t.Fatalf("atomic visit increment failed: data=%+v err=%v", visited, err)
	}
	if err := reopened.SetIfAbsent("persistent-code", data); !errors.Is(err, ErrKeyExists) {
		t.Fatalf("expected duplicate active code conflict, got %v", err)
	}
	if err := reopened.DeleteUser("persistent-user"); err != nil {
		t.Fatalf("delete persisted user: %v", err)
	}
	if _, err := reopened.Get("persistent-code"); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected user URL cascade deletion, got %v", err)
	}
}

func TestPostgresScopingAdminInvariantsAndCleanup(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testURL := isolatedPostgresURL(t, ctx, databaseURL)

	store, err := NewPostgresStore(ctx, testURL, "integration-admin-password", "admin@test.local")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	admin, err := store.GetUser(defaultAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateUserRole(defaultAdmin, models.RoleUser); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("expected last-admin demotion protection, got %v", err)
	}
	if err := store.DeleteUser(defaultAdmin); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("expected last-admin deletion protection, got %v", err)
	}
	if err := store.UpdateUserRole(defaultAdmin, "invalid"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("expected invalid role, got %v", err)
	}

	first, err := store.CreateUser("first-user", "first-password", "first@test.local")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateUser("second-user", "second-password", "second@test.local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser("first-user", "duplicate-password", "duplicate@test.local"); !errors.Is(err, ErrUsernameExists) {
		t.Fatalf("expected duplicate username, got %v", err)
	}
	if got, err := store.GetUserByID(first.ID); err != nil || got.Username != first.Username {
		t.Fatalf("lookup by ID failed: user=%+v err=%v", got, err)
	}
	users, err := store.GetAllUsers()
	if err != nil || len(users) != 3 {
		t.Fatalf("unexpected user list: count=%d err=%v", len(users), err)
	}

	now := time.Now()
	firstURL := &models.URLData{
		ID: utils.GenerateID(), ShortCode: "first-active", OriginalURL: "https://example.com/first",
		ExpiresIn: "24h", ExpiresAt: now.Add(time.Hour), CreatedAt: now, CreatedBy: first.ID,
	}
	secondURL := &models.URLData{
		ID: utils.GenerateID(), ShortCode: "second-active", OriginalURL: "https://example.com/second",
		ExpiresIn: "24h", ExpiresAt: now.Add(time.Hour), CreatedAt: now, CreatedBy: second.ID, Visits: 2,
	}
	expiredURL := &models.URLData{
		ID: utils.GenerateID(), ShortCode: "expired", OriginalURL: "https://example.com/expired",
		ExpiresIn: "5m", ExpiresAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Hour), CreatedBy: first.ID,
	}
	for _, data := range []*models.URLData{firstURL, secondURL, expiredURL} {
		if err := store.SetIfAbsent(data.ShortCode, data); err != nil {
			t.Fatal(err)
		}
	}

	firstURLs, err := store.GetAllFor(first.ID, models.RoleUser)
	if err != nil || len(firstURLs) != 1 {
		t.Fatalf("unexpected first-user URLs: count=%d err=%v", len(firstURLs), err)
	}
	adminURLs, err := store.GetAllFor(admin.ID, models.RoleAdmin)
	if err != nil || len(adminURLs) != 2 {
		t.Fatalf("unexpected admin URLs: count=%d err=%v", len(adminURLs), err)
	}
	stats, err := store.StatsFor(admin.ID, models.RoleAdmin)
	if err != nil || stats["total_urls"] != 2 || stats["total_visits"] != int64(2) {
		t.Fatalf("unexpected admin stats: stats=%v err=%v", stats, err)
	}
	if err := store.Delete("first-active"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("first-active"); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected missing delete, got %v", err)
	}

	replacement := *expiredURL
	replacement.ID = utils.GenerateID()
	replacement.OriginalURL = "https://example.com/replacement"
	replacement.ExpiresAt = now.Add(time.Hour)
	if err := store.SetIfAbsent("expired", &replacement); err != nil {
		t.Fatalf("replace expired URL: %v", err)
	}

	expiredForCleanup := &models.URLData{
		ID: utils.GenerateID(), ShortCode: "cleanup", OriginalURL: "https://example.com/cleanup",
		ExpiresIn: "5m", ExpiresAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Hour), CreatedBy: second.ID,
	}
	if err := store.SetIfAbsent("cleanup", expiredForCleanup); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.DeleteExpiredURLs(ctx, 10)
	if err != nil || deleted != 1 {
		t.Fatalf("unexpected cleanup result: deleted=%d err=%v", deleted, err)
	}

	if err := store.UpdateUserRole(first.Username, models.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateUserRole(defaultAdmin, models.RoleUser); err != nil {
		t.Fatalf("demote admin with replacement: %v", err)
	}
	if err := store.DeleteUser("missing-user"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected missing user delete, got %v", err)
	}
}

func TestPostgresRejectsMigrationChecksumMismatch(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testURL := isolatedPostgresURL(t, ctx, databaseURL)
	store, err := NewPostgresStore(ctx, testURL, "integration-admin-password", "admin@test.local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `
		UPDATE schema_migrations SET checksum = 'tampered' WHERE version = '001_init.sql'
	`); err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()

	reopened, err := NewPostgresStore(ctx, testURL, "integration-admin-password", "admin@test.local")
	if err == nil {
		reopened.Close()
		t.Fatal("expected migration checksum mismatch to prevent startup")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected migration integrity error: %v", err)
	}
}

func isolatedPostgresURL(t *testing.T, ctx context.Context, databaseURL string) string {
	t.Helper()

	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create integration database pool: %v", err)
	}

	// PostgreSQL folds unquoted identifiers to lowercase. Keeping the generated
	// schema lowercase and quoting it in both SQL and search_path makes the
	// isolation deterministic regardless of the random code's original case.
	schema := "goshorty_test_" + strings.ToLower(utils.GenerateShortCode(12))
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		adminPool.Close()
		t.Fatalf("create isolated integration schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := adminPool.Exec(cleanupCtx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop isolated integration schema: %v", err)
		}
		adminPool.Close()
	})

	testURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := testURL.Query()
	query.Set("search_path", identifier)
	testURL.RawQuery = query.Encode()
	return testURL.String()
}
