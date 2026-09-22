package services

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"goshorty/config"
	"goshorty/models"
	"goshorty/storage"
)

func TestURLInfoAndStatsRespectOwnership(t *testing.T) {
	store := storage.NewStorage()
	defer store.Stop()
	service := NewURLService(store, config.NewConfig())

	created, err := service.CreateShortURL(&models.ShortenRequest{
		URL: "https://example.com/private",
	}, "owner-id")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.GetURLInfo(created.ShortCode, "other-id", models.RoleUser); !errors.Is(err, ErrURLNotFound) {
		t.Fatalf("expected non-owner lookup to be hidden, got %v", err)
	}
	if _, err := service.GetURLInfo(created.ShortCode, "owner-id", models.RoleUser); err != nil {
		t.Fatalf("owner should read URL info: %v", err)
	}
	if _, err := service.GetURLInfo(created.ShortCode, "admin-id", models.RoleAdmin); err != nil {
		t.Fatalf("admin should read URL info: %v", err)
	}

	ownerStats, err := service.GetStats("owner-id", models.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	otherStats, err := service.GetStats("other-id", models.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	adminStats, err := service.GetStats("admin-id", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if ownerStats["total_urls"] != 1 || otherStats["total_urls"] != 0 || adminStats["total_urls"] != 1 {
		t.Fatalf("unexpected scoped stats: owner=%v other=%v admin=%v", ownerStats, otherStats, adminStats)
	}
}

func TestShortURLUsesConfiguredPublicBaseURL(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "https://short.example.com/")
	store := storage.NewStorage()
	defer store.Stop()
	service := NewURLService(store, config.NewConfig())

	created, err := service.CreateShortURL(&models.ShortenRequest{
		URL: "https://example.com",
	}, "owner-id")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.ShortURL, "https://short.example.com/r/") {
		t.Fatalf("unexpected public URL: %s", created.ShortURL)
	}
}

func TestConcurrentCustomCodeCreationHasSingleWinner(t *testing.T) {
	store := storage.NewStorage()
	defer store.Stop()
	service := NewURLService(store, config.NewConfig())

	const workers = 64
	var successes int32
	var conflicts int32
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, err := service.CreateShortURL(&models.ShortenRequest{
				URL:        "https://example.com",
				CustomCode: "atomic-code",
			}, "owner-id")
			switch {
			case err == nil:
				atomic.AddInt32(&successes, 1)
			case errors.Is(err, ErrCodeTaken):
				atomic.AddInt32(&conflicts, 1)
			default:
				t.Errorf("unexpected create error: %v", err)
			}
		}()
	}
	wg.Wait()

	if successes != 1 || conflicts != workers-1 {
		t.Fatalf("expected one success and %d conflicts, got success=%d conflicts=%d", workers-1, successes, conflicts)
	}
}
