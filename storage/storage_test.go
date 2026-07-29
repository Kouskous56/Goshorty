package storage

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"goshorty/models"
)

func TestSetIfAbsentConcurrent(t *testing.T) {
	store := NewStorage()
	defer store.Stop()

	const workers = 64
	var successes int32
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			data := &models.URLData{
				ID:          fmt.Sprintf("id-%d", i),
				ShortCode:   "shared",
				OriginalURL: fmt.Sprintf("https://example.com/%d", i),
				ExpiresAt:   time.Now().Add(time.Hour),
			}
			err := store.SetIfAbsent("shared", data)
			if err == nil {
				atomic.AddInt32(&successes, 1)
				return
			}
			if !errors.Is(err, ErrKeyExists) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly one successful reservation, got %d", successes)
	}
}

func TestStorageReturnsSnapshots(t *testing.T) {
	store := NewStorage()
	defer store.Stop()

	original := &models.URLData{
		ID:          "id-1",
		ShortCode:   "snapshot",
		OriginalURL: "https://example.com",
		ExpiresAt:   time.Now().Add(time.Hour),
		Visits:      1,
	}
	if err := store.SetIfAbsent("snapshot", original); err != nil {
		t.Fatal(err)
	}

	first, err := store.Get("snapshot")
	if err != nil {
		t.Fatal(err)
	}
	first.OriginalURL = "https://attacker.invalid"
	first.Visits = 999

	second, err := store.Get("snapshot")
	if err != nil {
		t.Fatal(err)
	}
	if second.OriginalURL != original.OriginalURL || second.Visits != original.Visits {
		t.Fatalf("caller mutation leaked into storage: %+v", second)
	}
}

func TestStorageStopIsIdempotent(t *testing.T) {
	store := NewStorage()
	store.Stop()
	store.Stop()
}

func TestLastAdminCannotBeRemoved(t *testing.T) {
	users, err := NewUserStorage("test-password", "admin@test.local")
	if err != nil {
		t.Fatal(err)
	}

	if err := users.UpdateUserRole("admin", models.RoleUser); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("expected ErrLastAdmin when demoting final admin, got %v", err)
	}
	if err := users.DeleteUser("admin"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("expected ErrLastAdmin when deleting final admin, got %v", err)
	}
}

func TestInMemoryURLLifecycleAndScoping(t *testing.T) {
	store := NewStorage()
	defer store.Stop()
	now := time.Now()
	ownerURL := &models.URLData{
		ID: "owner-url", ShortCode: "owner", OriginalURL: "https://example.com/owner",
		CreatedBy: "owner-id", CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	otherURL := &models.URLData{
		ID: "other-url", ShortCode: "other", OriginalURL: "https://example.com/other",
		CreatedBy: "other-id", CreatedAt: now, ExpiresAt: now.Add(time.Hour), Visits: 2,
	}
	expired := &models.URLData{
		ID: "expired-url", ShortCode: "expired", OriginalURL: "https://example.com/expired",
		CreatedBy: "owner-id", CreatedAt: now, ExpiresAt: now.Add(-time.Second),
	}
	for code, data := range map[string]*models.URLData{
		"owner": ownerURL, "other": otherURL, "expired": expired,
	} {
		if err := store.SetIfAbsent(code, data); err != nil {
			t.Fatal(err)
		}
	}

	ownerURLs, _ := store.GetAllFor("owner-id", models.RoleUser)
	adminURLs, _ := store.GetAllFor("admin-id", models.RoleAdmin)
	if len(ownerURLs) != 1 || len(adminURLs) != 2 {
		t.Fatalf("unexpected scoped lists: owner=%d admin=%d", len(ownerURLs), len(adminURLs))
	}
	stats, _ := store.StatsFor("admin-id", models.RoleAdmin)
	if stats["total_urls"] != 2 || stats["total_visits"] != int64(2) {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	if _, err := store.Get("expired"); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected expired URL to be hidden, got %v", err)
	}
	if _, err := store.GetAndIncrement("missing"); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected missing increment to fail, got %v", err)
	}
	if err := store.Delete("owner"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("owner"); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected second delete to fail, got %v", err)
	}
}

func TestInMemoryUserLifecycle(t *testing.T) {
	users, err := NewUserStorage("test-admin-password", "admin@test.local")
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.CreateUser("alice", "initial-password", "alice@test.local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.CreateUser("alice", "another-password", "other@test.local"); !errors.Is(err, ErrUsernameExists) {
		t.Fatalf("expected duplicate username, got %v", err)
	}
	if got, err := users.GetUserByID(user.ID); err != nil || got.Username != "alice" {
		t.Fatalf("lookup by ID failed: user=%+v err=%v", got, err)
	}
	if valid, err := users.VerifyPassword("alice", "initial-password"); err != nil || !valid {
		t.Fatalf("initial password failed: valid=%v err=%v", valid, err)
	}
	if err := users.UpdatePassword(user.ID, "replacement-password"); err != nil {
		t.Fatal(err)
	}
	updated, err := users.GetUserByID(user.ID)
	if err != nil || updated.TokenVersion != user.TokenVersion+1 {
		t.Fatalf("password change must increment token version: user=%+v err=%v", updated, err)
	}
	if valid, _ := users.VerifyPassword("alice", "replacement-password"); !valid {
		t.Fatal("replacement password should be valid")
	}
	beforeRevoke, _ := users.GetUserByID(user.ID)
	if err := users.RevokeTokens(user.ID); err != nil {
		t.Fatal(err)
	}
	afterRevoke, _ := users.GetUserByID(user.ID)
	if afterRevoke.TokenVersion != beforeRevoke.TokenVersion+1 {
		t.Fatalf("revoke must increment token version")
	}
	if err := users.UpdateUserRole("alice", "invalid"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("expected invalid role, got %v", err)
	}
	if err := users.UpdateUserRole("alice", models.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	all, err := users.GetAllUsers()
	if err != nil || len(all) != 2 {
		t.Fatalf("unexpected user list: count=%d err=%v", len(all), err)
	}
	if err := users.DeleteUser("alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := users.GetUser("alice"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected deleted user to be missing, got %v", err)
	}
}
