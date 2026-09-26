package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"goshorty/models"
	"goshorty/utils"
)

// TestPostgresListURLsPagination proves the keyset page query on the Postgres
// backend: ownership scoping, admin visibility, total, has-more detection and
// cursor-driven page walks without loading the whole table.
func TestPostgresListURLsPagination(t *testing.T) {
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

	owner, err := store.CreateUser("pagination-owner", "password-owner-1", "owner@test.local")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateUser("pagination-other", "password-other-1", "other@test.local")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	mk := func(code string, created time.Time, by string) *models.URLData {
		return &models.URLData{
			ID:          utils.GenerateID(),
			ShortCode:   code,
			OriginalURL: "https://example.com/pg/" + code,
			ExpiresIn:   "24h",
			ExpiresAt:   time.Now().Add(24 * time.Hour),
			CreatedAt:   created,
			CreatedBy:   by,
		}
	}
	for _, u := range []*models.URLData{
		mk("u1", base, owner.ID),
		mk("u2", base.Add(time.Second), owner.ID),
		mk("u3", base.Add(2*time.Second), owner.ID),
		mk("o1", base.Add(3*time.Second), other.ID),
	} {
		if err := store.SetIfAbsent(u.ShortCode, u); err != nil {
			t.Fatal(err)
		}
	}

	codes := func(items []*models.URLData) string {
		out := ""
		for _, u := range items {
			out += u.ShortCode
		}
		return out
	}

	// Owner sees only their own URLs, newest first: u3, u2, then u1.
	res, err := store.ListURLs(owner.ID, models.RoleUser, models.URLCursor{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := codes(res.Items); got != "u3u2" {
		t.Fatalf("page 1 = %q, want u3u2", got)
	}
	if res.Total != 3 || !res.HasMore {
		t.Fatalf("page 1 metadata: total=%d hasMore=%v", res.Total, res.HasMore)
	}

	last := res.Items[len(res.Items)-1]
	page2, err := store.ListURLs(owner.ID, models.RoleUser, models.URLCursor{
		CreatedAtUnixNano: last.CreatedAt.UnixNano(),
		ShortCode:         last.ShortCode,
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := codes(page2.Items); got != "u1" {
		t.Fatalf("page 2 = %q, want u1", got)
	}
	if page2.HasMore || page2.Total != 3 {
		t.Fatalf("page 2 metadata: total=%d hasMore=%v", page2.Total, page2.HasMore)
	}

	// Admin sees all four, including the foreign URL at the top.
	resA, err := store.ListURLs("", models.RoleAdmin, models.URLCursor{}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := codes(resA.Items); got != "o1u3u2" {
		t.Fatalf("admin page = %q, want o1u3u2", got)
	}
	if resA.Total != 4 || !resA.HasMore {
		t.Fatalf("admin metadata: total=%d hasMore=%v", resA.Total, resA.HasMore)
	}
}

// TestPostgresListUsersPagination proves the keyset page query for users:
// username ASC ordering, cursor walks and list metadata.
func TestPostgresListUsersPagination(t *testing.T) {
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

	for _, u := range []struct{ name, email string }{
		{"bravo", "bravo@test.local"},
		{"zeta", "zeta@test.local"},
		{"alpha", "alpha@test.local"},
	} {
		if _, err := store.CreateUser(u.name, "password-"+u.name, u.email); err != nil {
			t.Fatal(err)
		}
	}

	names := func(items []*models.User) []string {
		out := make([]string, 0, len(items))
		for _, u := range items {
			out = append(out, u.Username)
		}
		return out
	}

	page1, err := store.ListUsers(models.UserCursor{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(page1.Items); len(got) != 2 || got[0] != "admin" || got[1] != "alpha" {
		t.Fatalf("page 1 = %v, want [admin alpha]", got)
	}
	if page1.Total != 4 || !page1.HasMore {
		t.Fatalf("page 1 metadata: total=%d hasMore=%v", page1.Total, page1.HasMore)
	}

	page2, err := store.ListUsers(models.UserCursor{Username: "alpha"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(page2.Items); len(got) != 2 || got[0] != "bravo" || got[1] != "zeta" {
		t.Fatalf("page 2 = %v, want [bravo zeta]", got)
	}
	if page2.HasMore || page2.Total != 4 {
		t.Fatalf("page 2 metadata: total=%d hasMore=%v", page2.Total, page2.HasMore)
	}
}
