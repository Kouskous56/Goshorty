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
