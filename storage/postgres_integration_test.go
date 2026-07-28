package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

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
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer adminPool.Close()

	schema := "goshorty_test_" + utils.GenerateShortCode(12)
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, schema)); err != nil {
		t.Fatal(err)
	}
	defer adminPool.Exec(context.Background(), fmt.Sprintf(`DROP SCHEMA "%s" CASCADE`, schema))

	testURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := testURL.Query()
	query.Set("search_path", schema)
	testURL.RawQuery = query.Encode()

	store, err := NewPostgresStore(ctx, testURL.String(), "integration-admin-password", "admin@test.local")
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

	reopened, err := NewPostgresStore(ctx, testURL.String(), "integration-admin-password", "admin@test.local")
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
